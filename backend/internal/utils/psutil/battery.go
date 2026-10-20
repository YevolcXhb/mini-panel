//go:build linux

package psutil

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BatteryInfo 定义见 battery_info.go（无 build tag，跨平台共享）。

// batteryCandidateDirs 返回可能的 power_supply 电池目录（按优先级）。
// 优先 /sys/class/power_supply/battery；否则扫描全部 power_supply，
// 取 type 文件内容为 "Battery" 的目录。Android chroot 下还会尝试
// 宿主机逃逸根下的同名路径。
func batteryCandidateDirs() []string {
	bases := []string{"/sys/class/power_supply"}
	if root := androidHostRoot(); root != "" {
		bases = append([]string{filepath.Join(root, "sys/class/power_supply")}, bases...)
	}
	var dirs []string
	for _, base := range bases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		// 命名规范的 battery 目录排最前
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Name()), "battery") {
				dirs = append(dirs, filepath.Join(base, e.Name()))
			}
		}
		// 其它目录按 type==Battery 识别（如 max77759、bms 等 OPLUS/MTK 命名）
		for _, e := range entries {
			dir := filepath.Join(base, e.Name())
			if strings.Contains(strings.ToLower(e.Name()), "battery") {
				continue
			}
			if t, err := os.ReadFile(filepath.Join(dir, "type")); err == nil &&
				strings.EqualFold(strings.TrimSpace(string(t)), "Battery") {
				dirs = append(dirs, dir)
			}
		}
		if len(dirs) > 0 {
			return dirs
		}
	}
	return dirs
}

func readBatteryFile(dir, name string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func readBatteryFloat(dir, name string) (float64, bool) {
	s, ok := readBatteryFile(dir, name)
	if !ok {
		return 0, false
	}
	// 兼容 "4263" 与 "4263000 µV" 之类带单位尾巴
	fields := strings.Fields(s)
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// GetBatteryInfo 读取电池状态；找不到电池时 Present=false。
func GetBatteryInfo() *BatteryInfo {
	info := &BatteryInfo{}
	for _, dir := range batteryCandidateDirs() {
		// 目录必须真的暴露 capacity 才算可用电池
		capVal, ok := readBatteryFloat(dir, "capacity")
		if !ok {
			continue
		}
		info.Present = true
		info.Capacity = int(math.Round(capVal))
		info.Status, _ = readBatteryFile(dir, "status")
		info.Health, _ = readBatteryFile(dir, "health")
		info.Technology, _ = readBatteryFile(dir, "technology")

		// 单位自适应：内核 power_supply 规范要求 µV/µA/µAh，但 MTK/OPLUS
		// 等厂商驱动常直接输出 mV/mA/mAh（本机实测 voltage_now=4278 即 4.278V）。
		// 判定基准统一取「电压刻度」：µV 量级说明整套属性用内核微单位，
		// 电流/容量/功率都按微单位解析；否则按毫单位。这样电压、电流、功率
		// 三者不会因为各自数值大小不同而判到不同口径。
		microUnit := powerUnitIsMicro(dir)
		if v, ok := readBatteryFloat(dir, "voltage_now"); ok {
			info.VoltageV = normalizeVoltage(v)
		}
		if v, ok := readBatteryFloat(dir, "current_now"); ok {
			info.CurrentMA = normalizeCurrent(v, microUnit)
		}
		// 外接电源在线状态：usb / ac / charger 任一 online=1 即视为接着充电器。
		// 满电（status=Full）时 BMS 会断开充电回路，current_now 物理上就是 0，
		// 这时候算不出功耗是正常的，需要在 UI 上如实说明而不是显示 0.00 W。
		info.Charging = externalPowerOnline()
		// temp: 0.1℃
		if v, ok := readBatteryFloat(dir, "temp"); ok {
			info.TempC = v / 10
		}
		if v, ok := readBatteryFloat(dir, "charge_now"); ok {
			info.ChargeNowMAh = normalizeCharge(v, microUnit)
		}
		if v, ok := readBatteryFloat(dir, "charge_full"); ok {
			info.ChargeFullMAh = normalizeCharge(v, microUnit)
		}
		// 部分机型只用 batt_rm / batt_fcc（单位 mAh）。
		// 注意 charge_now 在 MTK 驱动里常是「电量计剩余电荷」，与满电设计容量
		// 口径不同（本机 5200/4000），因此当 charge_full 缺失或 charge_now
		// 明显超出 charge_full 时，改用 batt_rm/batt_fcc 这一对同口径值。
		rm, hasRM := readBatteryFloat(dir, "batt_rm")
		fcc, hasFCC := readBatteryFloat(dir, "batt_fcc")
		rawChargeMAh := info.ChargeNowMAh // 估算功耗必须用原始电量计读数
		needPair := !hasFCC || info.ChargeFullMAh == 0 ||
			(info.ChargeNowMAh > 0 && info.ChargeNowMAh > info.ChargeFullMAh*1.2)
		if needPair && hasRM && hasFCC && rm > 0 && fcc > 0 {
			info.ChargeNowMAh = rm
			info.ChargeFullMAh = fcc
		}
		if info.ChargeFullMAh <= 0 && info.ChargeNowMAh > 0 {
			info.ChargeFullMAh = info.ChargeNowMAh
		}
		if v, ok := readBatteryFloat(dir, "cycle_count"); ok && v >= 0 {
			info.CycleCount = int(v)
		}

		// 功耗三级取值：驱动功率节点 → V×I → mAh 变化率估算。
		//
		// 单位由同目录 voltage_now 的刻度推断：µV 量级说明驱动用内核标准
		// 微单位，则 power_now 按 µW 解析；否则按 mV/mW 降级口径解析。
		//
		// 关于「0 值」：驱动暴露 power_now 不等于它有稳定读数。本机满电插电时
		// current_now 在 0 / ±2.5mA 之间抖，power_now 随之在 0 与 ±11000µW 间跳。
		// 因此单次读到 0 不能作为有效功耗，改用多采样去抖（见 readPowerDebounced）。
		info.PowerSource = PowerSourceNone

		if v, ok := readPowerDebounced(dir, microUnit); ok {
			info.PowerW = v.watts
			info.PowerSource = PowerSourceDriver
			info.PowerSign = v.sign
		}

		// V × I 兜底：驱动没给出有效功率时，用电压电流算。
		// 驱动报的电流必须达到噪声门限才算真实电流 —— 本机满电时
		// current_now 会在 0 / ±2mA 之间抖动，2mA 折合 0.009W，
		// 低于 5mA 一律视为电量计噪声，不当成有效功耗。
		if info.PowerSource == PowerSourceNone && info.VoltageV > 0 && math.Abs(info.CurrentMA) >= currentNoiseFloorMA {
			info.PowerW = math.Abs(info.VoltageV * info.CurrentMA / 1000)
			info.PowerSource = PowerSourceVI
			// 电流符号：负=放电，正=充电
			if info.CurrentMA < 0 {
				info.PowerSign = PowerSignDischarge
			} else {
				info.PowerSign = PowerSignCharge
			}
		}
		// 兜底估算：仅当电池确实在放电（未接外部电源）时才尝试。
		// 满电/充电中 BMS 会断开或切换回路，charge_now 的跳变不代表放电，
		// 这时给出估算值就是编造数据，宁可返回 none。
		if info.PowerSource == PowerSourceNone && info.VoltageV > 0 && !info.Charging {
			if w, ok := estimatePowerFromCharge(rawChargeMAh, info.VoltageV); ok {
				info.PowerW = w
				info.PowerSource = PowerSourceEstimate
			}
		}
		return info
	}
	return info
}

// currentNoiseFloorMA 是电流噪声门限。低于这个值的 current_now 视为
// 电量计抖动（本机实测满电时会出现 ±2mA），不参与功耗计算。
const currentNoiseFloorMA = 5.0

// externalPowerOnline 检查 usb / ac / charger 任一节点 online==1。
// 这些节点与电池并列在 power_supply 下，路径同样要兼容 chroot 逃逸根。
func externalPowerOnline() bool {
	bases := []string{"/sys/class/power_supply"}
	if root := androidHostRoot(); root != "" {
		bases = append([]string{filepath.Join(root, "sys/class/power_supply")}, bases...)
	}
	for _, base := range bases {
		for _, name := range []string{"usb", "ac", "charger", "dc"} {
			if v, err := os.ReadFile(filepath.Join(base, name, "online")); err == nil {
				if strings.TrimSpace(string(v)) == "1" {
					return true
				}
			}
		}
	}
	return false
}

// estimateTracker 保存电量计历史采样，用于估算功耗。
//
// 关键约束：MTK 的 charge_now 是整数 mAh，满电静置时会因校准在
// 5206/5181 这类值之间**跳变**，直接用相邻两次差值除以间隔会算出
// 十几瓦的假功率。因此估算必须满足三个条件才给值：
//  1. 采样窗口足够长（≥60 秒），让量化误差 1mAh 摊薄到可接受的水平；
//  2. 窗口内电量单调同向变化（连续下降或连续上升），跳变被排除；
//  3. 算出的电流落在整机合理区间（20~8000mA），过大过小都丢弃。
type estimateTracker struct {
	mu      sync.Mutex
	samples []estimateSample // 最近的采样点（时间升序）
}

type estimateSample struct {
	atUnix int64
	mAh    float64
}

const (
	estimateWindowSec  = 60   // 采样窗口
	estimateMaxSamples = 64   // 环形上限
	estimateMinCurrent = 20.0 // 低于此电流视为静置/噪声
	estimateMaxCurrent = 8000.0
)

var chargeEstimate = &estimateTracker{}

// estimatePowerFromCharge 用长时间窗口内的电荷变化估算功耗。
//
// 原理：ΔQ(mAh) / Δt(h) = 平均电流 I(mA)，P = V × I / 1000。
// 窗口内所有相邻差值必须同号（全部非正或全部非负），
// 否则说明电量在来回跳，属于校准噪声，拒绝估算。
func estimatePowerFromCharge(chargeMAh, voltageV float64) (float64, bool) {
	if chargeMAh <= 0 || voltageV <= 0 {
		return 0, false
	}
	now := time.Now().Unix()
	t := chargeEstimate

	t.mu.Lock()
	defer t.mu.Unlock()

	// 追加采样并裁掉超出窗口的旧点
	t.samples = append(t.samples, estimateSample{atUnix: now, mAh: chargeMAh})
	for len(t.samples) > estimateMaxSamples {
		t.samples = t.samples[1:]
	}
	cut := 0
	for i, s := range t.samples {
		if now-s.atUnix <= estimateWindowSec {
			cut = i
			break
		}
	}
	t.samples = t.samples[cut:]

	first := t.samples[0]
	last := t.samples[len(t.samples)-1]
	span := last.atUnix - first.atUnix
	if span < estimateWindowSec/2 {
		return 0, false // 窗口还没攒够时长
	}
	// 单调性检查：窗口内每一步变化方向必须一致
	dir := 0
	for i := 1; i < len(t.samples); i++ {
		d := t.samples[i].mAh - t.samples[i-1].mAh
		if d == 0 {
			continue
		}
		s := 1
		if d < 0 {
			s = -1
		}
		if dir == 0 {
			dir = s
		} else if dir != s {
			return 0, false // 电量来回跳，校准噪声
		}
	}
	if dir == 0 {
		return 0, false // 整窗无变化
	}
	deltaMAh := math.Abs(last.mAh - first.mAh)
	currentMA := deltaMAh / (float64(span) / 3600.0)
	if currentMA < estimateMinCurrent || currentMA > estimateMaxCurrent {
		return 0, false
	}
	return math.Abs(voltageV * currentMA / 1000), true
}

// normalizeVoltage 把 voltage_now 归一化为伏特。
// µV 形态（>100000）除以 1e6；mV 形态（100~20000）除以 1000。
func normalizeVoltage(v float64) float64 {
	switch {
	case v > 100000:
		return v / 1e6 // µV
	case v > 100:
		return v / 1e3 // mV
	default:
		return v // 已是 V
	}
}

// normalizeCurrent 把 current_now 归一化为毫安。
//
// 单位不再靠数值大小猜。旧实现「|v|>10000 视为 µA」会误判：
// current_now=1000000 是 1000mA（µA 口径），但 10000 以下的大电流
// （如 3000 表示 3000mA）会被当成 mA 原样返回，量级差 1000 倍。
//
// 现在由调用方按同目录电压刻度给出 microUnit，与 power_now 的判断保持一致。
func normalizeCurrent(v float64, microUnit bool) float64 {
	if !microUnit {
		return v // 驱动整体降级为 mA
	}
	return v / 1e3 // µA → mA
}

// normalizeCharge 把 charge_now/charge_full 归一化为毫安时。
//
// 与电压/电流/功率统一：由 microUnit 决定口径，不再按单个数值大小猜。
// 旧实现用「>100000 视为 µAh」，本机 charge_now=4000000（µAh=4000mAh）
// 会被误判，且 4000mAh 的 mAh 口径（4000）与 µAh 口径边界混淆。
func normalizeCharge(v float64, microUnit bool) float64 {
	if !microUnit {
		return v // 驱动整体降级为 mAh
	}
	return v / 1e3 // µAh → mAh
}

// powerUnitIsMicro 判断 power_supply 驱动属性是否使用内核标准微单位。
//
// 依据同目录 voltage_now 的量级：内核规范是 µV（手机电池约 4.2e6），
// 厂商驱动常整体降级为 mV（约 4200）。据此决定 power_now 按 µW 还是 mW 解析，
// 避免「数值落在两种解释都合理」的区间时判错。读不到电压时按内核标准处理。
func powerUnitIsMicro(dir string) bool {
	if v, ok := readBatteryFloat(dir, "voltage_now"); ok && v > 0 {
		return v > 100000
	}
	return true
}

// powerReading 一次去抖后的功率读数
type powerReading struct {
	watts float64      // 绝对值（W）
	sign  string       // charge / discharge / unknown
}

// powerDebounceSamples 去抖采样次数。
//
// 本机实测：驱动 power_now 的零值率约 55%（连续 40 次读数中 25 次为 0），
// 因为它在两次电流采样之间会返回 0。按独立事件估算，全零概率为 0.55^N：
//   N=5 → 5.0%（实测 6 次里偶有 2 次全零）
//   N=9 → 0.46%
// 9 次读取的开销可忽略（每次只是读一个 sysfs 小文件），但稳定性提升一个量级。
const powerDebounceSamples = 9

// readPowerDebounced 读取 power_now（缺失时退回 power_avg）并做去抖。
//
// 策略：
//  1. 连续读 N 次，解析成瓦特（带符号）
//  2. 丢弃 0 值 —— 驱动在采样间隙会返回 0，那不是"功耗为零"而是"本轮无读数"
//  3. 取剩余值的**中位数**，抗单点毛刺
//  4. 若全部为 0 或全部读不到，返回 ok=false，交给上层走 V×I 兜底
//
// 返回的 sign 由中位数的符号决定：负=放电，正=充电/输入。
func readPowerDebounced(dir string, microUnit bool) (powerReading, bool) {
	attr := "power_now"
	if _, ok := readBatteryFloat(dir, attr); !ok {
		attr = "power_avg"
	}

	vals := make([]float64, 0, powerDebounceSamples)
	var rawNonZero int
	for i := 0; i < powerDebounceSamples; i++ {
		raw, ok := readBatteryFloat(dir, attr)
		if !ok {
			continue
		}
		if raw == 0 {
			continue // 采样间隙，跳过
		}
		rawNonZero++
		signed := raw
		if microUnit {
			signed = raw / 1e6
		} else {
			signed = raw / 1e3
		}
		vals = append(vals, signed)
	}
	if len(vals) == 0 {
		return powerReading{}, false
	}
	_ = rawNonZero

	// 中位数（按有符号值排序，保持方向一致后再取）
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]

	// 幅度用绝对值的中位数更稳：先取绝对值排序再取中位
	absSorted := make([]float64, len(vals))
	for i, v := range vals {
		absSorted[i] = math.Abs(v)
	}
	sort.Float64s(absSorted)
	absMedian := absSorted[len(absSorted)/2]

	sign := PowerSignUnknown
	switch {
	case median < 0:
		sign = PowerSignDischarge
	case median > 0:
		sign = PowerSignCharge
	}

	return powerReading{watts: absMedian, sign: sign}, true
}

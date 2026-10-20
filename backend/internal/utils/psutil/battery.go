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
		// 供电形态：决定「功耗为 0」是物理事实还是数据缺失。
		if info.Charging {
			info.PowerMode = PowerModeUSB
		} else {
			info.PowerMode = PowerModeBattery
		}
		// temp: 0.1℃
		if v, ok := readBatteryFloat(dir, "temp"); ok {
			info.TempC = v / 10
		}
		// 容量：不能套用电压刻度。
		//
		// 本机实测：voltage_now=4416000（µV 口径），但 charge_now=5193、
		// charge_full=4000、batt_rm=batt_fcc=4000 全是 mAh 口径。
		// 同一个驱动里 voltage 用微单位、charge 用毫单位 —— 强行统一会
		// 把 4000mAh 再除以 1000 变成 4mAh（界面上就是 "0 / 4 mAh"）。
		//
		// 所以容量单独按数值量级判断：手机电池容量在 1000~20000mAh 之间，
		// µAh 表示会是 1e6~2e7 量级，两者有 1000 倍差距，不会混淆。
		if v, ok := readBatteryFloat(dir, "charge_now"); ok {
			info.ChargeNowMAh = normalizeChargeByMagnitude(v)
		}
		if v, ok := readBatteryFloat(dir, "charge_full"); ok {
			info.ChargeFullMAh = normalizeChargeByMagnitude(v)
		}
		// 部分机型只用 batt_rm / batt_fcc（单位 mAh），同样按量级归一。
		// 注意 charge_now 在 MTK 驱动里常是「电量计剩余电荷」，与满电设计容量
		// 口径不同（本机实测 5168~5218 之间跳，而 charge_full 恒为 4000），
		// 因此当 charge_full 缺失或 charge_now 明显超出 charge_full 时，
		// 改用 batt_rm/batt_fcc 这一对同口径值。
		rm, hasRM := readBatteryFloat(dir, "batt_rm")
		fcc, hasFCC := readBatteryFloat(dir, "batt_fcc")
		rmMAh := normalizeChargeByMagnitude(rm)
		fccMAh := normalizeChargeByMagnitude(fcc)
		rawChargeMAh := info.ChargeNowMAh // 估算功耗必须用原始电量计读数
		needPair := !hasFCC || info.ChargeFullMAh == 0 ||
			(info.ChargeNowMAh > 0 && info.ChargeNowMAh > info.ChargeFullMAh*1.2)
		if needPair && hasRM && hasFCC && rmMAh > 0 && fccMAh > 0 {
			info.ChargeNowMAh = rmMAh
			info.ChargeFullMAh = fccMAh
		}

		// 电量计读数缺失时的兜底。
		//
		// Type-C 小板直供的形态下，驱动不走充电流程，电量计可能返回
		// charge_now=0 或停在某个旧值不动。此时界面显示 "0 / 4000 mAh"
		// 会让人误以为电池空了 —— 实际是「没有读数」。
		//
		// 用 capacity（百分比）乘以满电容量推算剩余电量，并标记为推算值，
		// 前端可据此显示约等号。capacity 由 BMS 维护，比 charge_now 可靠。
		if info.ChargeFullMAh > 0 {
			switch {
			case info.ChargeNowMAh <= 0:
				// 完全没有读数：按百分比推算
				if info.Capacity > 0 {
					info.ChargeNowMAh = info.ChargeFullMAh * float64(info.Capacity) / 100
					info.ChargeNowEstimated = true
				}
			case info.ChargeNowMAh > info.ChargeFullMAh:
				// 读数超过满电容量（电量计口径偏大）：按百分比推算更可信
				if info.Capacity > 0 {
					info.ChargeNowMAh = info.ChargeFullMAh * float64(info.Capacity) / 100
					info.ChargeNowEstimated = true
				}
			}
		}
		if info.ChargeFullMAh <= 0 && info.ChargeNowMAh > 0 {
			info.ChargeFullMAh = info.ChargeNowMAh
		}
		if v, ok := readBatteryFloat(dir, "cycle_count"); ok && v >= 0 {
			info.CycleCount = int(v)
		}

		// 功耗计算：按供电形态选择数据源（见 readSystemPower）。
		//
		// 本机形态说明：电池是经 Type-C 供电的「模拟小板」接入主板电池接口，
		// 没有真实电芯。所以：
		//   - usb.online=1：外部通过 usb 供电路径给整机，输入侧功率才是整机功耗
		//   - usb.online=0：电池接口侧（小板输出）就是整机唯一的电源
		//
		// 单位由同目录 voltage_now 的刻度推断：µV 量级说明驱动用内核标准
		// 微单位，则 power_now 按 µW 解析；否则按 mV/mW 降级口径解析。
		info.PowerSource = PowerSourceNone

		if sys, ok := readSystemPower(dir, info, microUnit); ok {
			info.PowerW = sys.watts
			info.PowerSource = sys.source
			info.PowerSign = sys.sign
			info.PowerDetail = sys.detail
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

// systemPower 是一次系统功耗计算的结果
type systemPower struct {
	watts  float64 // 瓦特，恒为正
	source string  // PowerSource* 常量
	sign   string  // PowerSign* 常量
	detail string  // 计算口径说明，给前端展示
}

// readSystemPower 按供电形态选择功耗数据源。
//
// 取数逻辑（对本机「电池 = Type-C 供电的模拟小板」这一形态）：
//
//	usb.online == 1  →  system_power = usb.power_now        （充电器输入侧）
//	usb.online == 0  →  system_power = battery.power_now    （电池接口侧）
//
//	若插着 USB 且电池侧还在放电（小板未满电），总功耗要加上放电部分：
//	  system_power = usb.power_now
//	               + max(0, -battery.current_now × battery.voltage_now / 1e6)
//
// 注意最后一项的单位：current_now 是 µA、voltage_now 是 µV，
// 两者相乘再除以 1e6 得到 µW，与 usb.power_now 的 µW 口径一致。
//
// usb.power_now 不存在时（内核未暴露），退回 battery 侧计算。
func readSystemPower(dir string, info *BatteryInfo, microUnit bool) (systemPower, bool) {
	usbDir := siblingSupplyDir(dir, "usb")

	// 1) usb 输入侧：优先用 V × I 自算，power_now 只作交叉校验。
	//
	// 为什么不信 usb/power_now：本机实测它有多级缓存，刷新不规律，
	// 会出现 5.2V × 62.5mA 却报 13mW 这种明显不自洽的值（应为 325mW）。
	// 而 voltage_now × current_now 在同一时刻是自洽的：
	//   空闲 5.19V × 35mA ≈ 0.18W
	//   满载 5.08V × 380mA ≈ 1.93W
	// 所以 V×I 是主路径，power_now 仅在被 V×I 判为无效时兜底。
	//
	// 单位必须按 usb 自己的 voltage_now 判断，不能沿用 battery 的 ——
	// 两者是不同口径（本机 battery 报 mV 而 usb 报 µV）。
	//
	// 另外要区分两个测量点，不能混淆：
	//   usb/voltage_now     = 适配器进线电压（约 5.2V，经过充电 IC 之前）
	//   battery/voltage_now = 电芯电压（约 4.28V，充电 IC 降压之后）
	// 算整机从适配器取电要用前者。
	var usbWatts float64
	hasUSB := false
	usbSrc := PowerSourceVI
	if usbDir != "" {
		usbMicro := powerUnitIsMicro(usbDir)
		usbSource := PowerSourceVI
		if w, ok := usbVI(usbDir, usbMicro); ok && w > 0 {
			usbWatts = w
			hasUSB = true
		} else if v, ok := readPowerDebounced(usbDir, usbMicro); ok && v.watts > 0 {
			// V×I 拿不到（电流长期为 0）时才退回 power_now
			usbWatts = v.watts
			hasUSB = true
			usbSource = PowerSourceDriver
		}
		if hasUSB {
			usbSrc = usbSource
		}
	}

	if hasUSB {
		// 2) 插着 USB 时，若电池侧仍在放电（小板未满电），加上放电功率
		batteryDischarge := 0.0
		if info.CurrentMA < 0 {
			// current_ma 已是 mA，voltage_v 已是 V，相乘得到 W
			batteryDischarge = math.Abs(info.VoltageV * info.CurrentMA / 1000)
		}
		total := usbWatts
		detail := "USB 输入侧（V×I）"
		if usbSrc == PowerSourceDriver {
			detail = "USB 输入侧（驱动）"
		}
		if batteryDischarge > 0.001 {
			total += batteryDischarge
			detail = "USB 输入 + 电池放电"
		}
		return systemPower{
			watts:  total,
			source: usbSrc,
			sign:   PowerSignCharge,
			detail: detail,
		}, true
	}

	// 3) 没有 usb 侧功率：按电池接口侧计算
	if v, ok := readPowerDebounced(dir, microUnit); ok && v.watts > 0 {
		sign := v.sign
		// 驱动的 power_now 符号约定不统一，有时恒为正。此时若电流明确为负，
		// 应以电流方向为准 —— 电池接口侧负电流就是放电。
		if sign == PowerSignUnknown || (sign == PowerSignCharge && info.CurrentMA < 0) {
			if info.CurrentMA < 0 {
				sign = PowerSignDischarge
			} else if info.CurrentMA > 0 {
				sign = PowerSignCharge
			}
		}
		return systemPower{
			watts:  v.watts,
			source: PowerSourceDriver,
			sign:   sign,
			detail: "电池接口侧",
		}, true
	}

	// 4) 最后兜底：V×I。门限按形态区分 ——
	//    USB 供电时电池不放电，只有 ±2mA 噪声，必须过滤；
	//    电池供电时电池就是整机电源，1mA 以上的读数都可能是真实负载。
	noiseFloor := currentNoiseFloorMA
	if !info.Charging {
		noiseFloor = 1.0
	}
	if info.VoltageV > 0 && math.Abs(info.CurrentMA) >= noiseFloor {
		sign := PowerSignCharge
		if info.CurrentMA < 0 {
			sign = PowerSignDischarge
		}
		return systemPower{
			watts:  math.Abs(info.VoltageV * info.CurrentMA / 1000),
			source: PowerSourceVI,
			sign:   sign,
			detail: "电压 × 电流",
		}, true
	}
	return systemPower{}, false
}

// usbVI 用 usb 节点的 voltage_now × current_now 算输入功率（W）。
//
// 单位：内核规范是 µV / µA，相乘后除以 1e12 得 W；
// 厂商降级成 mV / mA 时除以 1e6。microUnit 指示是否用微单位。
//
// 两个属性都必须先去抖再相乘。实测 usb/current_now 在 0 与数十 mA 之间跳，
// 直接成对相乘会得到大量 0W 假读数（电流为 0 时乘积恒为 0）。
// 因此分别取电压与电流的中位数，再相乘 —— 这样某一项偶发为 0 不会污染结果。
func usbVI(usbDir string, microUnit bool) (float64, bool) {
	const samples = 9

	voltages := make([]float64, 0, samples)
	currents := make([]float64, 0, samples)

	for i := 0; i < samples; i++ {
		if v, ok := readBatteryFloat(usbDir, "voltage_now"); ok && v > 0 {
			voltages = append(voltages, v)
		}
		if c, ok := readBatteryFloat(usbDir, "current_now"); ok && c != 0 {
			// 取绝对值：输入侧电流方向对"取了多少电"没有意义
			currents = append(currents, math.Abs(c))
		}
	}
	if len(voltages) == 0 || len(currents) == 0 {
		return 0, false
	}

	sort.Float64s(voltages)
	sort.Float64s(currents)
	v := voltages[len(voltages)/2]
	c := currents[len(currents)/2]

	if c == 0 {
		return 0, false
	}

	if microUnit {
		return v * c / 1e12, true // µV × µA → W
	}
	return v * c / 1e6, true // mV × mA → W
}

// siblingSupplyDir 返回同级的另一个 power_supply 设备目录。
// power_supply 目录本身可能是符号链接，所以用 Glob 而不是 ReadDir。
func siblingSupplyDir(batteryDir, name string) string {
	bases := []string{"/sys/class/power_supply"}
	if root := androidHostRoot(); root != "" {
		bases = append([]string{filepath.Join(root, "sys/class/power_supply")}, bases...)
	}
	// 先看 battery 所在目录的上级（通常是同一个 power_supply）
	if parent := filepath.Dir(batteryDir); parent != "." && parent != "/" {
		cand := filepath.Join(parent, name)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	for _, base := range bases {
		cand := filepath.Join(base, name)
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

// externalPowerOnline 检查 usb / ac / charger 任一节点 online==1。
// 这些节点与电池并列在 power_supply 下，路径同样要兼容 chroot 逃逸根。
//
// 注意：Type-C 小板直供的机器上 usb.online 会是 0 —— 那时电池就是整机的
// 供电源，功耗必须靠 V×I 或电量变化率算，不能当作「无读数」。
func externalPowerOnline() bool {
	return powerSupplyOnline("usb", "ac", "charger", "dc")
}

// powerSupplyOnline 检查指定 power_supply 节点中是否有 online==1。
func powerSupplyOnline(names ...string) bool {
	bases := []string{"/sys/class/power_supply"}
	if root := androidHostRoot(); root != "" {
		bases = append([]string{filepath.Join(root, "sys/class/power_supply")}, bases...)
	}
	for _, base := range bases {
		for _, name := range names {
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

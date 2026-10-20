//go:build linux

package psutil

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		// 按数量级判定：电压 >100 视为 mV，电流/容量同理。
		if v, ok := readBatteryFloat(dir, "voltage_now"); ok {
			info.VoltageV = normalizeVoltage(v)
		}
		if v, ok := readBatteryFloat(dir, "current_now"); ok {
			info.CurrentMA = normalizeCurrent(v)
		}
		// 优先使用驱动原生 power_now；为空/0 时回退 V×I
		// （该平台无 power_now 与 powercap，功耗只能由 V×I 计算，
		//   current_now 在满电/空闲态为 0 属物理正常）
		if p, ok := readBatteryFloat(dir, "power_now"); ok && p > 0 {
			info.PowerW = p / 1e6
		}
		if info.PowerW == 0 && info.VoltageV > 0 && info.CurrentMA != 0 {
			info.PowerW = math.Abs(info.VoltageV * info.CurrentMA / 1000)
		}
		// temp: 0.1℃
		if v, ok := readBatteryFloat(dir, "temp"); ok {
			info.TempC = v / 10
		}
		if v, ok := readBatteryFloat(dir, "charge_now"); ok {
			info.ChargeNowMAh = normalizeCharge(v)
		}
		if v, ok := readBatteryFloat(dir, "charge_full"); ok {
			info.ChargeFullMAh = normalizeCharge(v)
		}
		// 部分机型只用 batt_rm / batt_fcc（单位 mAh）
		if info.ChargeNowMAh == 0 {
			if v, ok := readBatteryFloat(dir, "batt_rm"); ok && v > 0 {
				info.ChargeNowMAh = v
			}
		}
		if info.ChargeFullMAh == 0 {
			if v, ok := readBatteryFloat(dir, "batt_fcc"); ok && v > 0 {
				info.ChargeFullMAh = v
			}
		}
		if v, ok := readBatteryFloat(dir, "cycle_count"); ok && v >= 0 {
			info.CycleCount = int(v)
		}
		return info
	}
	return info
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
// µA 形态（|v|>10000）除以 1e3；mA 形态原样返回。
func normalizeCurrent(v float64) float64 {
	if math.Abs(v) > 10000 {
		return v / 1e3 // µA
	}
	return v // mA
}

// normalizeCharge 把 charge_now/charge_full 归一化为毫安时。
// µAh 形态（>100000）除以 1e3；mAh 形态原样返回。
func normalizeCharge(v float64) float64 {
	if v > 100000 {
		return v / 1e3 // µAh
	}
	return v // mAh
}

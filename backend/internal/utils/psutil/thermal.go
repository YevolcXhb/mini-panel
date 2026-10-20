//go:build linux

package psutil

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ThermalZone 单个温度传感器（/sys/class/thermal/thermal_zoneN/）。
// TempC 由毫摄氏度 ÷1000 换算（如 31146 → 31.146℃）。
type ThermalZone struct {
	Index int     `json:"index"`
	Type  string  `json:"type"`  // 传感器名，如 mtktscpu
	TempC float64 `json:"temp_c"`
	Valid bool    `json:"valid"` // false = 无效占位值（如 -127℃ 的 tzimgs）
}

// invalidTempThreshold 无效占位判定：驱动未接入的传感器（如摄像头 tzimgs）
// 用 -127000（-127℃）甚至更低填充，普通环境不可能出现，直接过滤。
const invalidTempThresholdC = -50.0

// androidHostRoot 定义于 android_linux.go（宿主逃逸根）。

// thermalBaseDirs 返回可能的 thermal class 目录（Android 下优先宿主逃逸根）。
func thermalBaseDirs() []string {
	bases := []string{"/sys/class/thermal"}
	if root := androidHostRoot(); root != "" {
		bases = append([]string{filepath.Join(root, "sys/class/thermal")}, bases...)
	}
	return bases
}

// GetThermalZones 枚举全部 thermal_zone 传感器温度。
// temp 为毫摄氏度 ÷1000；负值低于 -50℃ 视为无效占位（valid=false），
// 默认只在 valid 传感器中返回—— tzimgs 之类 -127 噪声不再出现。
func GetThermalZones() []ThermalZone {
	var zones []ThermalZone
	seen := map[string]bool{}
	for _, base := range thermalBaseDirs() {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "thermal_zone") {
				continue
			}
			dir := filepath.Join(base, e.Name())
			idx := 0
			if _, tail, ok := strings.Cut(e.Name(), "thermal_zone"); ok {
				idx, _ = strconv.Atoi(tail)
			}
			zType := readFirstLine(filepath.Join(dir, "type"))
			zTemp, rawOK := readThermalTemp(filepath.Join(dir, "temp"))
			valid := rawOK && zTemp > invalidTempThresholdC && zTemp < 150
			key := zType + "|" + strconv.Itoa(idx)
			if seen[key] {
				continue
			}
			seen[key] = true
			if !valid {
				continue // 无效占位（-127 等）直接丢弃，不进列表
			}
			zones = append(zones, ThermalZone{Index: idx, Type: zType, TempC: zTemp, Valid: true})
		}
		if len(zones) > 0 {
			break // 第一个可用的 base（优先宿主逃逸根）已经拿到数据
		}
	}
	return zones
}

// GetCpuTemp 取 CPU 温度：按 type 名找（mtktscpu 等，zone 编号重启会变，
// 不能硬编码），优先级 cpu > soc > 最高温的有效传感器。
func GetCpuTemp() (float64, bool) {
	zones := GetThermalZones()
	if len(zones) == 0 {
		return 0, false
	}
	for _, kw := range []string{"cpu", "soc", "ap"} {
		for _, z := range zones {
			if strings.Contains(strings.ToLower(z.Type), kw) {
				return z.TempC, true
			}
		}
	}
	best := zones[0]
	for _, z := range zones[1:] {
		if z.TempC > best.TempC {
			best = z
		}
	}
	return best.TempC, true
}

func readThermalTemp(path string) (float64, bool) {
	s := readFirstLine(path)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v / 1000.0, true // 毫摄氏度 → ℃
}

func readFirstLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

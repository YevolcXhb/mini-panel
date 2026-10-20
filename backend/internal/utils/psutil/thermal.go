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
}

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
// type 优先取 zone 目录下的 type 文件；temp 为毫摄氏度 ÷1000。
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
			// 以 type+temp 作为去重键，chroot 与宿主逃逸根可能重叠
			idx := 0
			if _, tail, ok := strings.Cut(e.Name(), "thermal_zone"); ok {
				idx, _ = strconv.Atoi(tail)
			}
			zType := readFirstLine(filepath.Join(dir, "type"))
			zTemp := readThermalTemp(filepath.Join(dir, "temp"))
			key := zType + "|" + strconv.FormatFloat(zTemp, 'f', 3, 64)
			if seen[key] {
				continue
			}
			seen[key] = true
			if zType == "" && zTemp == 0 {
				continue
			}
			zones = append(zones, ThermalZone{Index: idx, Type: zType, TempC: zTemp})
		}
		if len(zones) > 0 {
			break // 第一个可用的 base（优先宿主逃逸根）已经拿到数据
		}
	}
	return zones
}

// GetCpuTemp 取 CPU 温度：优先 type 含 cpu 的传感器，否则取温度最高的一个。
func GetCpuTemp() (float64, bool) {
	zones := GetThermalZones()
	if len(zones) == 0 {
		return 0, false
	}
	for _, z := range zones {
		if strings.Contains(strings.ToLower(z.Type), "cpu") {
			return z.TempC, true
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

func readThermalTemp(path string) float64 {
	s := readFirstLine(path)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v / 1000.0 // 毫摄氏度 → ℃
}

func readFirstLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

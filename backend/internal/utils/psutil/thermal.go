//go:build linux

package psutil

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ThermalZone 单个温度传感器（/sys/class/thermal/thermal_zoneN/）。
// TempC 由毫摄氏度 ÷1000 换算（如 31146 → 31.146℃）。
//
// 注意：sysfs 下 thermal_zoneN 是符号链接，DirEntry.IsDir() 对其返回 false，
// 因此必须用 filepath.Glob 枚举路径，不能用 ReadDir + IsDir 过滤。
type ThermalZone struct {
	Index int     `json:"index"`
	Type  string  `json:"type"`  // 传感器名，如 mtktscpu
	TempC float64 `json:"temp_c"`
	Valid bool    `json:"valid"` // false = 无效占位值（如 -127℃ 的 tzimgs）
}

// invalidTempThresholdC 无效占位判定：未接入的传感器（如摄像头 tzimgs）
// 由驱动用 -127000（-127℃）填充，普通环境不可能出现，直接过滤。
const invalidTempThresholdC = -50.0

// thermalMaxTempC 上限合理性判断。
const thermalMaxTempC = 150.0

// androidHostRoot 定义于 android_linux.go（宿主逃逸根）。

// thermalBaseDirs 返回可能的 thermal class 目录（Android 下优先宿主逃逸根）。
func thermalBaseDirs() []string {
	bases := []string{"/sys/class/thermal"}
	if root := androidHostRoot(); root != "" {
		bases = append([]string{filepath.Join(root, "sys/class/thermal")}, bases...)
	}
	return bases
}

// GetThermalZones 枚举全部有效 thermal_zone 传感器温度。
// 使用 filepath.Glob 枚举（sysfs 条目是符号链接，ReadDir 的 IsDir 恒为 false）；
// temp 为毫摄氏度 ÷1000；低于 -50℃ 或高于 150℃ 视为无效占位，直接丢弃。
func GetThermalZones() []ThermalZone {
	var zones []ThermalZone
	for _, base := range thermalBaseDirs() {
		matches, err := filepath.Glob(filepath.Join(base, "thermal_zone*"))
		if err != nil || len(matches) == 0 {
			continue
		}
		sort.Slice(matches, func(i, j int) bool {
			return zoneIndexOf(matches[i]) < zoneIndexOf(matches[j])
		})
		for _, dir := range matches {
			if _, err := os.Stat(dir); err != nil {
				continue // 悬空链接
			}
			zType := readFirstLine(filepath.Join(dir, "type"))
			zTemp, ok := readThermalTemp(filepath.Join(dir, "temp"))
			if !ok || zTemp <= invalidTempThresholdC || zTemp >= thermalMaxTempC {
				continue // 无效占位（tzimgs 的 -127）或读不到
			}
			zones = append(zones, ThermalZone{
				Index: zoneIndexOf(dir),
				Type:  zType,
				TempC: zTemp,
				Valid: true,
			})
		}
		if len(zones) > 0 {
			break // 第一个可用的 base（优先宿主逃逸根）已经拿到数据
		}
	}
	return zones
}

// GetCpuTemp 取 CPU 温度：按 type 名匹配（zone 编号重启会变，不能硬编码），
// 优先级 cpu > soc > ap，都没有则取有效传感器中温度最高的。
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

func zoneIndexOf(dir string) int {
	name := filepath.Base(dir)
	if _, tail, ok := strings.Cut(name, "thermal_zone"); ok {
		if v, err := strconv.Atoi(tail); err == nil {
			return v
		}
	}
	return 0
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

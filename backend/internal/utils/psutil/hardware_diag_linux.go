//go:build linux

package psutil

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BatteryDiag / ThermalDiag 的诊断信息（用于排查面板在 Android chroot
// 下读不到硬件数据时的路径选择问题）。
type HardwareDiag struct {
	HostRoot     string   `json:"host_root"`      // 探测到的 Android 宿主根（空=未逃逸）
	ChrootLike   bool     `json:"chroot_like"`    // 是否判定为 Android chroot 环境
	BatteryDirs  []string `json:"battery_dirs"`   // 实际尝试的电池目录
	BatteryHit   string   `json:"battery_hit"`    // 实际命中的电池目录
	ThermalBases []string `json:"thermal_bases"`  // 实际尝试的 thermal 基目录
	ThermalHit   string   `json:"thermal_hit"`    // 实际命中的 thermal 基目录
	ThermalRaw   []string `json:"thermal_raw"`    // 原始读到的 zone（含无效占位，未过滤）
	ThermalValid int      `json:"thermal_valid"`  // 有效传感器数量
	BatteryFiles []string `json:"battery_files"`  // 命中电池目录下可见的属性文件
}

// GetHardwareDiag 采集硬件读取路径诊断信息。
func GetHardwareDiag() *HardwareDiag {
	d := &HardwareDiag{
		HostRoot:     androidHostRoot(),
		ChrootLike:   looksLikeAndroidChroot(),
		BatteryDirs:  batteryCandidateDirs(),
		ThermalBases: thermalBaseDirs(),
	}

	for _, dir := range d.BatteryDirs {
		if _, ok := readBatteryFloat(dir, "capacity"); ok {
			d.BatteryHit = dir
			entries, err := os.ReadDir(dir)
			if err == nil {
				var names []string
				for _, e := range entries {
					if !e.IsDir() {
						names = append(names, e.Name())
					}
				}
				sort.Strings(names)
				d.BatteryFiles = names
			}
			break
		}
	}

	// 原始 thermal（不过滤无效值），用于定位 -127 占位噪声
	for _, base := range d.ThermalBases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		var raw []string
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "thermal_zone") {
				continue
			}
			dir := filepath.Join(base, e.Name())
			t := readFirstLine(filepath.Join(dir, "type"))
			v, ok := readThermalTemp(filepath.Join(dir, "temp"))
			if ok {
				raw = append(raw, t+"="+strings.TrimRight(strings.TrimRight(
					formatFloat(v), "0"), ".")+"C")
			} else {
				raw = append(raw, t+"=<unreadable>")
			}
		}
		if len(raw) > 0 {
			d.ThermalHit = base
			d.ThermalRaw = raw
			break
		}
	}
	d.ThermalValid = len(GetThermalZones())
	return d
}

func formatFloat(v float64) string {
	// 保留 3 位小数，避免依赖 strconv 引入额外 import
	neg := v < 0
	if neg {
		v = -v
	}
	i := int64(v)
	frac := int64((v-float64(i))*1000 + 0.5)
	s := itoa64(i) + "." + pad3(frac)
	if neg {
		return "-" + s
	}
	return s
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	var b [24]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

func pad3(v int64) string {
	s := itoa64(v)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

//go:build !linux

package psutil

// HardwareDiag 非 Linux 平台诊断信息（保持 API 形状一致）。
type HardwareDiag struct {
	HostRoot     string   `json:"host_root"`
	ChrootLike   bool     `json:"chroot_like"`
	BatteryDirs  []string `json:"battery_dirs"`
	BatteryHit   string   `json:"battery_hit"`
	ThermalBases []string `json:"thermal_bases"`
	ThermalHit   string   `json:"thermal_hit"`
	ThermalRaw   []string `json:"thermal_raw"`
	ThermalValid int      `json:"thermal_valid"`
	BatteryFiles []string `json:"battery_files"`
}

// GetHardwareDiag 非 Linux 平台无硬件接口，返回空诊断。
func GetHardwareDiag() *HardwareDiag {
	return &HardwareDiag{}
}

//go:build !linux

package psutil

// ThermalZone 非 Linux 平台的空温度传感器结构（保持 API 形状一致）。
type ThermalZone struct {
	Index int     `json:"index"`
	Type  string  `json:"type"`
	TempC float64 `json:"temp_c"`
}

// GetThermalZones 非 Linux 平台无 thermal class，返回空列表。
func GetThermalZones() []ThermalZone {
	return nil
}

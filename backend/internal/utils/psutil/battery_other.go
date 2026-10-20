//go:build !linux

package psutil

// GetBatteryInfo 非 Linux 平台无电池接口，返回未检测状态。
func GetBatteryInfo() *BatteryInfo {
	return &BatteryInfo{}
}

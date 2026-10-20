package psutil

// BatteryInfo 电池硬件信息（Android power_supply class）。
// PowerW = |VoltageV × CurrentA|，为计算值而非直接读取。
// 结构体无 build tag，Linux 实现在 battery.go，非 Linux 桩在 battery_other.go。
type BatteryInfo struct {
	Present       bool    `json:"present"`
	Capacity      int     `json:"capacity"`        // 电量百分比 0-100
	Status        string  `json:"status"`          // Charging / Discharging / Not charging / Full
	VoltageV      float64 `json:"voltage_v"`       // 电压（V）
	CurrentMA     float64 `json:"current_ma"`      // 电流（mA，符号表示充放电方向）
	PowerW        float64 `json:"power_w"`         // 功率（W）
	TempC         float64 `json:"temp_c"`          // 温度（℃）
	Health        string  `json:"health"`          // Good / Overheat / Dead ...
	Technology    string  `json:"technology"`      // 电池技术（Li-poly 等）
	ChargeNowMAh  float64 `json:"charge_now_mah"`  // 当前电荷（mAh）
	ChargeFullMAh float64 `json:"charge_full_mah"` // 满充电荷（mAh）
	CycleCount    int     `json:"cycle_count"`     // 循环次数
}

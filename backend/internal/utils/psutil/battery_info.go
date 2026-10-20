package psutil

// BatteryInfo 电池硬件信息（Android power_supply class）。
//
// PowerW 的含义要分清「读到的」和「算出来的」两层：
//   - PowerSource 描述这个值从哪来：driver / vi / estimate / none
//   - 驱动直接暴露功率节点时用它；否则 P = V × I；电流为 0 且电池在放电时，
//     用 mAh 变化率估算（见 EstimatePowerFromCharge）。
//
// 结构体无 build tag，Linux 实现在 battery.go，非 Linux 桩在 battery_other.go。
type BatteryInfo struct {
	Present       bool    `json:"present"`
	Capacity      int     `json:"capacity"`        // 电量百分比 0-100
	Status        string  `json:"status"`          // Charging / Discharging / Not charging / Full
	VoltageV      float64 `json:"voltage_v"`       // 电压（V）
	CurrentMA     float64 `json:"current_ma"`      // 电流（mA，符号表示充放电方向）
	PowerW        float64 `json:"power_w"`         // 功率（W），0 表示当前读不到
	PowerSource   string  `json:"power_source"`    // driver / vi / estimate / none
	Charging      bool    `json:"charging"`        // 是否接入外部电源（usb/charger/ac online）
	TempC         float64 `json:"temp_c"`          // 温度（℃）
	Health        string  `json:"health"`          // Good / Overheat / Dead ...
	Technology    string  `json:"technology"`      // 电池技术（Li-poly 等）
	ChargeNowMAh  float64 `json:"charge_now_mah"`  // 当前电荷（mAh）
	ChargeFullMAh float64 `json:"charge_full_mah"` // 满充电荷（mAh）
	CycleCount    int     `json:"cycle_count"`     // 循环次数
}

// 功率来源常量
const (
	PowerSourceDriver   = "driver"   // 驱动 power_now 直接读取
	PowerSourceVI       = "vi"       // 电压 × 电流计算
	PowerSourceEstimate = "estimate" // mAh 变化率估算
	PowerSourceNone     = "none"     // 读不到（满电静置 / 无电流采样）
)

// estimateChargeTracker 保存上一次的电荷读数，用于估算功耗。
// 由于 GetBatteryInfo 在每次 API 请求时调用，两次调用间的电荷差
// 除以时间差即为平均电流，再乘电压得到估算功率。
type estimateChargeTracker struct {
	lastChargeMAh float64
	lastAt        int64 // Unix 秒
}

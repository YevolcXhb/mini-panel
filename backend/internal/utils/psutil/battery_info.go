package psutil

import "math"

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
	PowerW        float64 `json:"power_w"`         // 功率（W，恒为正），0 表示当前读不到
	PowerSource   string  `json:"power_source"`    // driver / vi / estimate / none
	PowerSign     string  `json:"power_sign"`      // charge / discharge / unknown：功率方向
	Charging      bool    `json:"charging"`        // 是否接入外部电源（usb/charger/ac online）
	TempC         float64 `json:"temp_c"`          // 温度（℃）
	Health        string  `json:"health"`          // Good / Overheat / Dead ...
	Technology    string  `json:"technology"`      // 电池技术（Li-poly 等）
	ChargeNowMAh  float64 `json:"charge_now_mah"`  // 当前电荷（mAh）
	ChargeFullMAh float64 `json:"charge_full_mah"` // 满充电荷（mAh）
	// ChargeNowEstimated 为 true 表示 charge_now_mah 是按 capacity 百分比
	// 推算出来的，而不是电量计的直接读数。仅在电量计不可信时置位
	// （直供形态 charge_now=0，或读数超过满电容量）。
	ChargeNowEstimated bool `json:"charge_now_estimated"`
	CycleCount         int  `json:"cycle_count"` // 循环次数
	// PowerMode 供电形态，用于解释「为什么功耗是 0」：
	//   usb      —— usb.online=1，整机由 USB 供电，电池侧功率天然接近 0
	//   battery  —— usb/ac/charger 全 offline，电池就是供电来源（Type-C 小板直供）
	//   取值见 PowerModeUSB / PowerModeBattery
	PowerMode string `json:"power_mode"`
}

// 功率来源常量
const (
	PowerSourceDriver   = "driver"   // 驱动 power_now 直接读取
	PowerSourceVI       = "vi"       // 电压 × 电流计算
	PowerSourceEstimate = "estimate" // mAh 变化率估算
	PowerSourceNone     = "none"     // 读不到（满电静置 / 无电流采样）
)

// 功率方向常量。
//
// 各驱动对 power_now 的正负约定并不统一，所以这里不假设固定语义，
// 而是由读数符号推断：负=电池放电，正=充电/外部输入，无法判定时 unknown。
const (
	PowerSignCharge    = "charge"
	PowerSignDischarge = "discharge"
	PowerSignUnknown   = "unknown"
)

// 供电形态常量。
//
// 直接影响功耗的可读性：
//   - USB 供电时，电池不参与放电，battery/power_now 只反映漏电流；
//   - 电池供电（Type-C 小板直供）时，电池就是整机电源，
//     功耗应当由电流推导，是真实可用的读数。
const (
	PowerModeUSB     = "usb"     // 接外部电源
	PowerModeBattery = "battery" // 电池供电
)

// estimateChargeTracker 保存上一次的电荷读数，用于估算功耗。
// 由于 GetBatteryInfo 在每次 API 请求时调用，两次调用间的电荷差
// 除以时间差即为平均电流，再乘电压得到估算功率。
type estimateChargeTracker struct {
	lastChargeMAh float64
	lastAt        int64 // Unix 秒
}

// normalizePowerWithHint 把 power_now/power_avg 归一化为瓦特（恒为正）。
//
// 单位歧义无法只靠数值大小解决：12000 既可能是 12000 µW（0.012W），
// 也可能是 12000 mW（12W），两种都在手机功耗的可能范围内。所以由调用方
// 根据同目录的属性单位给出提示 microUnit：
//   - 内核 power_supply 规范：voltage_now 用 µV（手机电池约 4.2e6）、
//     current_now 用 µA、power_now 用 µW
//   - 部分厂商驱动整体降级成 mV / mA / mW（voltage_now 约 4200）
//
// 提示判断不了时（如电压读不到），按内核标准 µW 处理。
// primary 超出合理区间时用另一种解释兜底，避免个别驱动的怪值直接漏出去。
func normalizePowerWithHint(v float64, microUnit bool) float64 {
	a := math.Abs(v)
	if a == 0 {
		return 0
	}

	primary, secondary := a/1e6, a/1e3 // µW 解释 / mW 解释
	if !microUnit {
		primary, secondary = secondary, primary
	}

	// 手机整机功耗大致在 1mW ~ 60W
	const lo, hi = 0.001, 60.0
	if primary >= lo && primary <= hi {
		return primary
	}
	if secondary >= lo && secondary <= hi {
		return secondary
	}
	return primary
}

// normalizeChargeByMagnitude 把容量归一化为毫安时。
//
// 容量不能像电压/电流那样用 voltage 刻度判单位：本机实测同一个驱动里
// voltage_now=4416000（µV）而 charge_full=4000（mAh），两者口径不同。
// 强行统一会把 4000mAh 除以 1000 变成 4mAh —— 界面上就是「0 / 4 mAh」。
//
// 改用数值量级判断 —— 手机电池容量在 100~20000mAh，µAh 表示会是
// 1e5~2e7，两个区间相差 1000 倍且不重叠，判断可靠。
// 放在无 build tag 的文件里，便于跨平台单测。
func normalizeChargeByMagnitude(v float64) float64 {
	if v <= 0 {
		return 0
	}
	if v > 100000 {
		return v / 1e3 // µAh → mAh
	}
	return v // mAh
}

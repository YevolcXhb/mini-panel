//go:build linux

package psutil

import (
	"testing"
	"time"
)

// 用本机真实的 /sys 验证单位推断与解析链路。
// 在没有 power_supply 的环境（普通 x86 Linux 容器）自动跳过。
func TestRealSysfsPowerUnits(t *testing.T) {
	dirs := batteryCandidateDirs()
	if len(dirs) == 0 {
		t.Skip("本机没有 power_supply/battery，跳过")
	}
	dir := dirs[0]
	t.Logf("真实电池目录: %s", dir)

	v, ok := readBatteryFloat(dir, "voltage_now")
	if !ok || v <= 0 {
		t.Skipf("读不到 voltage_now（%v, ok=%v），跳过", v, ok)
	}
	t.Logf("voltage_now 原始值 = %v", v)

	// 单位推断必须与电压量级自洽
	micro := powerUnitIsMicro(dir)
	var volts float64
	if micro {
		volts = v / 1e6
	} else {
		volts = v / 1e3
	}
	t.Logf("判定为微单位=%v，换算电压=%.3f V", micro, volts)

	// 锂离子电池电压合理区间
	if volts < 3.0 || volts > 4.6 {
		t.Fatalf("电压换算不合理：%.3f V（原始 %v，micro=%v）", volts, v, micro)
	}

	// 若内核已暴露 power_now，检查能否给出合理瓦数
	if p, ok := readBatteryFloat(dir, "power_now"); ok {
		w := normalizePowerWithHint(p, micro)
		t.Logf("power_now 原始值 = %v，换算 = %.6f W", p, w)
		if w > 60 {
			t.Fatalf("power_now 换算结果不合理：%v W", w)
		}
	} else {
		t.Log("内核尚未暴露 power_now")
	}

	// 电流同理，确认符号与量级可读
	if c, ok := readBatteryFloat(dir, "current_now"); ok {
		var ma float64
		if micro {
			ma = c / 1e3
		} else {
			ma = c
		}
		t.Logf("current_now 原始值 = %v（微单位=%v → %.3f mA）", c, micro, ma)
	}
}

// 端到端验证：面板的 GetBatteryInfo 在内核暴露 power_now 后，
// 是否走 driver 直读分支、瓦数是否合理。
func TestRealBatteryInfoEndToEnd(t *testing.T) {
	if len(batteryCandidateDirs()) == 0 {
		t.Skip("本机没有 power_supply/battery，跳过")
	}
	info := GetBatteryInfo()
	if info == nil {
		t.Fatal("GetBatteryInfo 返回 nil")
	}
	t.Logf("present=%v capacity=%d%% status=%s", info.Present, info.Capacity, info.Status)
	t.Logf("voltage=%.3fV current=%.3fmA", info.VoltageV, info.CurrentMA)
	t.Logf("power=%.6fW source=%s charging=%v", info.PowerW, info.PowerSource, info.Charging)
	t.Logf("temp=%.1fC health=%s charge=%.0f/%.0fmAh", info.TempC, info.Health, info.ChargeNowMAh, info.ChargeFullMAh)

	// 电压必须归一化到伏特
	if info.VoltageV < 3.0 || info.VoltageV > 4.6 {
		t.Fatalf("电压未正确归一化: %.3f V", info.VoltageV)
	}
	// 功率来源必须是已知取值之一
	switch info.PowerSource {
	case PowerSourceDriver, PowerSourceVI, PowerSourceEstimate, PowerSourceNone:
	default:
		t.Fatalf("未知的 power_source: %s", info.PowerSource)
	}
	// 若走了 driver 直读，瓦数必须在合理区间
	if info.PowerSource == PowerSourceDriver {
		if info.PowerW < 0 || info.PowerW > 60 {
			t.Fatalf("driver 功率不合理: %v W", info.PowerW)
		}
	}
}

// 观察 power_now 直读的波动特征：驱动值在 0 与负值之间跳，
// 直接透传会让前端显示闪烁。这里量化波动幅度，为平滑策略提供依据。
func TestPowerNowFluctuation(t *testing.T) {
	if len(batteryCandidateDirs()) == 0 {
		t.Skip("没有 power_supply/battery")
	}
	dir := batteryCandidateDirs()[0]
	micro := powerUnitIsMicro(dir)

	var vals []float64
	for i := 0; i < 10; i++ {
		if p, ok := readBatteryFloat(dir, "power_now"); ok {
			vals = append(vals, normalizePowerWithHint(p, micro))
		}
		time.Sleep(300 * time.Millisecond)
	}
	if len(vals) == 0 {
		t.Skip("读不到 power_now")
	}

	minV, maxV, zeros := vals[0], vals[0], 0
	for _, v := range vals {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		if v == 0 {
			zeros++
		}
	}
	t.Logf("power_now 采样 %d 次: min=%.6fW max=%.6fW 零值=%d 次", len(vals), minV, maxV, zeros)
	t.Logf("原始序列: %v", vals)

	if maxV > 0 && (maxV-minV)/maxV > 0.5 {
		t.Logf("波动幅度 %.0f%%，超过 50%%，前端直接透传会明显闪烁",
			(maxV-minV)/maxV*100)
	}
}

//go:build linux

package psutil

import (
	"math"
	"testing"
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
		t.Logf("power_now 原始值 = %v，换算 = %.3f W", p, w)
		if w > 60 {
			t.Fatalf("power_now 换算结果不合理：%v W", w)
		}
	} else {
		t.Log("内核尚未暴露 power_now（这正是当前要补的节点）")
	}

	// 电流同理，确认符号与量级可读
	if c, ok := readBatteryFloat(dir, "current_now"); ok {
		t.Logf("current_now 原始值 = %v（微单位=%v → %.3f mA）", c, micro, func() float64 {
			if micro {
				return c / 1e3
			}
			return c
		}())
	}
	_ = math.Abs
}

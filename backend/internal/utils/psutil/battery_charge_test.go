package psutil

import "testing"

// 容量单位必须按数值量级判断，不能套用电压刻度。
//
// 本机实测同一个 power_supply 驱动里：voltage_now=4416000（µV），
// charge_now=5193、charge_full=4000、batt_rm=batt_fcc=4000（全是 mAh）。
// 若强行按电压刻度把容量除以 1000，4000mAh 会变成 4mAh ——
// 界面上就是 "0 / 4 mAh" 那种错误显示。
func TestNormalizeChargeByMagnitude(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		// 本机真实值：mAh 口径
		{"本机 charge_now", 5193, 5193},
		{"本机 charge_full", 4000, 4000},
		{"本机 batt_rm", 4000, 4000},
		{"本机 batt_fcc", 4000, 4000},

		// µAh 口径（内核标准，量级大 1000 倍）
		{"µAh 4000mAh", 4000000, 4000},
		{"µAh 5000mAh", 5000000, 5000},
		{"µAh 5193mAh", 5193000, 5193},

		// 边界与异常
		{"零", 0, 0},
		{"负值视为无效", -100, 0},
		{"小容量 500mAh", 500, 500},
		{"大容量 20000mAh", 20000, 20000},
	}

	for _, c := range cases {
		got := normalizeChargeByMagnitude(c.in)
		if got != c.want {
			t.Errorf("%s: normalizeChargeByMagnitude(%v) = %v，期望 %v",
				c.name, c.in, got, c.want)
		}
	}
}

// 回归：本机 4000mAh 绝不能被算成 4mAh
func TestChargeFullNotDividedWrongly(t *testing.T) {
	got := normalizeChargeByMagnitude(4000)
	if got < 1000 {
		t.Fatalf("4000 应解释为 4000mAh，得到 %v —— 单位判断回归", got)
	}
}

// µAh 与 mAh 两个区间不能重叠误判
func TestChargeUnitRangesDoNotOverlap(t *testing.T) {
	// 最大手机容量 mAh 口径 20000，最小 µAh 口径约 1e5
	mAhMax := normalizeChargeByMagnitude(20000)
	uAhMin := normalizeChargeByMagnitude(100001)

	if mAhMax != 20000 {
		t.Fatalf("20000 应原样保留，得到 %v", mAhMax)
	}
	if uAhMin < 100 || uAhMin > 101 {
		t.Fatalf("100001µAh 应约等于 100mAh，得到 %v", uAhMin)
	}
}

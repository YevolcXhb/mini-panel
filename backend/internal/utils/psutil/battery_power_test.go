package psutil

import (
	"math"
	"testing"
)

// normalizePowerWithHint 要同时正确处理内核 µW 与厂商 mW 两种口径，
// 且不受正负号影响（放电为负的驱动很常见）。
func TestNormalizePowerWithHint(t *testing.T) {
	cases := []struct {
		name  string
		in    float64
		micro bool
		want  float64
	}{
		// 内核标准 µW（voltage_now 为 4.2e6 量级时）
		{"µW 0.48W", 480000, true, 0.48},
		{"µW 4.8W", 4800000, true, 4.8},
		{"µW 20W", 20000000, true, 20},
		{"µW 0.1W", 100000, true, 0.1},
		{"µW 50mW 低功耗", 50000, true, 0.05},
		{"µW 10mW 下限", 10000, true, 0.01},
		{"负值 µW 0.48W", -480000, true, 0.48},
		{"负值 µW 8W", -8000000, true, 8},

		// 厂商 mW（voltage_now 为 4200 量级时）
		{"mW 0.48W", 480, false, 0.48},
		{"mW 4.8W", 4800, false, 4.8},
		{"mW 12W 快充", 12000, false, 12},
		{"mW 30W 快充", 30000, false, 30},
		{"负值 mW 3W", -3000, false, 3},

		// 零
		{"零", 0, true, 0},
		{"零 mW", 0, false, 0},
	}

	for _, c := range cases {
		got := normalizePowerWithHint(c.in, c.micro)
		if math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: normalizePowerWithHint(%v, micro=%v) = %v，期望 %v",
				c.name, c.in, c.micro, got, c.want)
		}
	}
}

// 同一数值在两种口径下必须给出不同的结果 —— 这正是需要单位提示的原因。
// 12000 若按 µW 是 0.012W，按 mW 是 12W，两者都可能出现，不能靠数值猜。
func TestNormalizePowerHintChangesResult(t *testing.T) {
	const v = 12000
	microRes := normalizePowerWithHint(v, true)
	milliRes := normalizePowerWithHint(v, false)

	if math.Abs(microRes-0.012) > 1e-9 {
		t.Errorf("按 µW 应得 0.012W，实际 %v", microRes)
	}
	if math.Abs(milliRes-12) > 1e-9 {
		t.Errorf("按 mW 应得 12W，实际 %v", milliRes)
	}
	if microRes == milliRes {
		t.Fatal("单位提示未生效，两种口径算出同一结果")
	}
}

// 旧实现的回归：50mW 的 µW 表示是 50000，曾被当成 mW 再除 1000 算成 50W。
func TestNormalizePowerLowPowerRegression(t *testing.T) {
	got := normalizePowerWithHint(50000, true)
	if got > 1 {
		t.Fatalf("50mW 被算成 %v W（应约 0.05W），低功耗单位判断回归", got)
	}
	if math.Abs(got-0.05) > 1e-9 {
		t.Fatalf("50mW 应为 0.05W，实际 %v", got)
	}
}

// 结果恒为正，前端可直接展示
func TestNormalizePowerAlwaysPositive(t *testing.T) {
	for _, v := range []float64{-1, -1000, -480000, -5000000} {
		if got := normalizePowerWithHint(v, true); got < 0 {
			t.Fatalf("输入 %v 得到负值 %v", v, got)
		}
	}
}

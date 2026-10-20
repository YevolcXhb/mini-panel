//go:build linux

package psutil

import (
	"os"
	"path/filepath"
	"testing"
)

// 伪造 power_now 序列的测试辅助：写一个固定值，读一次；用于验证去抖逻辑。
func fakeBatteryWith(t *testing.T, attrs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for k, v := range attrs {
		if err := os.WriteFile(filepath.Join(dir, k), []byte(v+"\n"), 0o644); err != nil {
			t.Fatalf("写入 %s 失败: %v", k, err)
		}
	}
	return dir
}

// 恒定非零值：去抖后应给出该值
func TestReadPowerDebouncedStableValue(t *testing.T) {
	dir := fakeBatteryWith(t, map[string]string{
		"voltage_now": "4279000",
		"power_now":   "11042", // 11.042 mW
	})
	got, ok := readPowerDebounced(dir, true)
	if !ok {
		t.Fatal("恒定非零值应能读出")
	}
	if got.watts < 0.0109 || got.watts > 0.0112 {
		t.Fatalf("期望约 0.011042W，实际 %v", got.watts)
	}
	if got.sign != PowerSignCharge {
		t.Fatalf("正值应为 charge，实际 %s", got.sign)
	}
}

// 全零：应返回 ok=false，交给上层走 V×I 兜底
func TestReadPowerDebouncedAllZero(t *testing.T) {
	dir := fakeBatteryWith(t, map[string]string{
		"voltage_now": "4279000",
		"power_now":   "0",
	})
	if _, ok := readPowerDebounced(dir, true); ok {
		t.Fatal("全零读数不应作为有效功耗返回")
	}
}

// 负值：方向应为 discharge，幅度为正
func TestReadPowerDebouncedNegativeIsDischarge(t *testing.T) {
	dir := fakeBatteryWith(t, map[string]string{
		"voltage_now": "4279000",
		"power_now":   "-11042",
	})
	got, ok := readPowerDebounced(dir, true)
	if !ok {
		t.Fatal("负值应能读出")
	}
	if got.watts < 0 {
		t.Fatalf("幅度必须为正，实际 %v", got.watts)
	}
	if got.sign != PowerSignDischarge {
		t.Fatalf("负值应为 discharge，实际 %s", got.sign)
	}
}

// 缺少 power_now 时必须回退到 power_avg
func TestReadPowerDebouncedFallsBackToAvg(t *testing.T) {
	dir := fakeBatteryWith(t, map[string]string{
		"voltage_now": "4279000",
		"power_avg":   "22000",
	})
	got, ok := readPowerDebounced(dir, true)
	if !ok {
		t.Fatal("应回退到 power_avg")
	}
	if got.watts < 0.021 || got.watts > 0.023 {
		t.Fatalf("期望约 0.022W，实际 %v", got.watts)
	}
}

// 两个节点都不存在：返回 false
func TestReadPowerDebouncedNoAttr(t *testing.T) {
	dir := fakeBatteryWith(t, map[string]string{"voltage_now": "4279000"})
	if _, ok := readPowerDebounced(dir, true); ok {
		t.Fatal("无功率节点时不应返回有效读数")
	}
}

// 厂商 mW 口径同样要正确处理
func TestReadPowerDebouncedMilliUnit(t *testing.T) {
	dir := fakeBatteryWith(t, map[string]string{
		"voltage_now": "4279", // mV
		"power_now":   "12000", // 12W
	})
	got, ok := readPowerDebounced(dir, false)
	if !ok {
		t.Fatal("应能读出")
	}
	if got.watts < 11.99 || got.watts > 12.01 {
		t.Fatalf("mW 口径下 12000 应为 12W，实际 %v", got.watts)
	}
}

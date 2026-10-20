//go:build linux

package psutil

import (
	"os"
	"path/filepath"
	"testing"
)

// 在临时目录里伪造一个 power_supply/battery 节点，验证真实读取路径：
// 包括单位提示判断、power_now 直读、以及与 V×I 的优先级关系。
func writeFakeBattery(t *testing.T, attrs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for k, v := range attrs {
		if err := os.WriteFile(filepath.Join(dir, k), []byte(v+"\n"), 0o644); err != nil {
			t.Fatalf("写入 %s 失败: %v", k, err)
		}
	}
	return dir
}

// 内核标准口径：voltage_now 为 µV 量级 → power_now 必须按 µW 解析
func TestPowerUnitIsMicroKernelStandard(t *testing.T) {
	dir := writeFakeBattery(t, map[string]string{
		"voltage_now": "4279000", // 4.279 V，µV
		"power_now":   "480000",  // 0.48 W，µW
	})

	if !powerUnitIsMicro(dir) {
		t.Fatal("voltage_now=4279000 应判定为微单位")
	}
	p, ok := readBatteryFloat(dir, "power_now")
	if !ok {
		t.Fatal("应能读到 power_now")
	}
	if got := normalizePowerWithHint(p, true); got < 0.479 || got > 0.481 {
		t.Fatalf("power_now=480000 应解析为 0.48W，实际 %v", got)
	}
}

// 厂商降级口径：voltage_now 为 mV 量级 → power_now 按 mW 解析
func TestPowerUnitIsMicroVendorMilli(t *testing.T) {
	dir := writeFakeBattery(t, map[string]string{
		"voltage_now": "4279",  // 4.279 V，mV
		"power_now":   "12000", // 12 W，mW
	})

	if powerUnitIsMicro(dir) {
		t.Fatal("voltage_now=4279 应判定为毫单位")
	}
	p, _ := readBatteryFloat(dir, "power_now")
	if got := normalizePowerWithHint(p, false); got < 11.99 || got > 12.01 {
		t.Fatalf("power_now=12000 在 mW 口径下应为 12W，实际 %v", got)
	}
}

// 电压读不到时按内核标准（µW）处理
func TestPowerUnitIsMicroDefaultsToMicro(t *testing.T) {
	dir := writeFakeBattery(t, map[string]string{"power_now": "480000"})
	if !powerUnitIsMicro(dir) {
		t.Fatal("缺少 voltage_now 时应默认按微单位处理")
	}
}

// 放电为负的驱动也必须能取到值（旧实现条件 p>0 会直接丢弃）
func TestPowerNowNegativeIsAccepted(t *testing.T) {
	dir := writeFakeBattery(t, map[string]string{
		"voltage_now": "4279000",
		"power_now":   "-3500000", // 放电 3.5W
	})
	p, ok := readBatteryFloat(dir, "power_now")
	if !ok {
		t.Fatal("负值 power_now 应能读到")
	}
	if p >= 0 {
		t.Fatalf("应读到负值，实际 %v", p)
	}
	if got := normalizePowerWithHint(p, true); got < 3.49 || got > 3.51 {
		t.Fatalf("负值 -3500000 应解析为 3.5W，实际 %v", got)
	}
}

//go:build linux

package psutil

import (
	"os"
	"path/filepath"
	"testing"
)

// usb 节点的单位必须独立判断，不能沿用 battery 的刻度。
//
// 本机实测：battery/voltage_now=4278000（µV），usb/voltage_now=5181250（µV），
// 两者恰好都是微单位，但代码不能依赖这个巧合 —— 厂商驱动可能一个用 mV
// 一个用 µV。这里用一个「battery 是 mV、usb 是 µV」的混合场景来锁死行为。
func TestUSBPowerUsesOwnUnitScale(t *testing.T) {
	root := t.TempDir()

	// battery：mV 口径（4278 = 4.278V）
	bt := filepath.Join(root, "battery")
	os.MkdirAll(bt, 0o755)
	os.WriteFile(filepath.Join(bt, "voltage_now"), []byte("4278\n"), 0o644)
	os.WriteFile(filepath.Join(bt, "current_now"), []byte("0\n"), 0o644)
	os.WriteFile(filepath.Join(bt, "power_now"), []byte("0\n"), 0o644)

	// usb：µV 口径，power_now = 250000 µW = 0.25W
	ub := filepath.Join(root, "usb")
	os.MkdirAll(ub, 0o755)
	os.WriteFile(filepath.Join(ub, "online"), []byte("1\n"), 0o644)
	os.WriteFile(filepath.Join(ub, "voltage_now"), []byte("5181250\n"), 0o644)
	os.WriteFile(filepath.Join(ub, "current_now"), []byte("48250\n"), 0o644)
	os.WriteFile(filepath.Join(ub, "power_now"), []byte("250000\n"), 0o644)

	info := &BatteryInfo{Charging: true, VoltageV: 4.278, CurrentMA: 0}
	got, ok := readSystemPower(bt, info, false) // battery 按毫单位
	if !ok {
		t.Fatal("应能算出功耗")
	}
	if got.watts < 0.249 || got.watts > 0.251 {
		t.Fatalf("usb.power_now=250000µW 应为 0.25W，实际 %v（单位判错？）", got.watts)
	}
}

// usb.power_now 缺失时，用 usb 的 voltage_now × current_now 兜底
func TestUSBPowerFallsBackToVI(t *testing.T) {
	root := t.TempDir()
	bt := filepath.Join(root, "battery")
	os.MkdirAll(bt, 0o755)
	os.WriteFile(filepath.Join(bt, "voltage_now"), []byte("4278000\n"), 0o644)
	os.WriteFile(filepath.Join(bt, "current_now"), []byte("0\n"), 0o644)

	ub := filepath.Join(root, "usb")
	os.MkdirAll(ub, 0o755)
	os.WriteFile(filepath.Join(ub, "online"), []byte("1\n"), 0o644)
	os.WriteFile(filepath.Join(ub, "voltage_now"), []byte("5200000\n"), 0o644) // 5.2V
	os.WriteFile(filepath.Join(ub, "current_now"), []byte("40000\n"), 0o644)  // 40mA
	// 故意不写 power_now

	info := &BatteryInfo{Charging: true, VoltageV: 4.278, CurrentMA: 0}
	got, ok := readSystemPower(bt, info, true)
	if !ok {
		t.Fatal("usb VI 兜底应生效")
	}
	// 5.2V × 40mA = 0.208W
	if got.watts < 0.20 || got.watts > 0.22 {
		t.Fatalf("usb V×I 应约 0.208W，实际 %v", got.watts)
	}
}

// usbVI 单位换算：µV×µA 与 mV×mA 两种口径
func TestUSBVIUnits(t *testing.T) {
	cases := []struct {
		name      string
		v, c      string
		microUnit bool
		want      float64
	}{
		{"µV×µA 5.19V 35mA", "5190000", "35000", true, 5.19 * 35 / 1000},
		{"µV×µA 5.08V 380mA", "5080000", "380000", true, 5.08 * 380 / 1000},
		{"mV×mA 5190 35", "5190", "35", false, 5.19 * 35 / 1000},
	}
	for _, c := range cases {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "voltage_now"), []byte(c.v+"\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "current_now"), []byte(c.c+"\n"), 0o644)
		got, ok := usbVI(dir, c.microUnit)
		if !ok {
			t.Fatalf("%s: 应能算出", c.name)
		}
		if got < c.want-0.01 || got > c.want+0.01 {
			t.Fatalf("%s: 期望 %.4fW，实际 %.4fW", c.name, c.want, got)
		}
	}
}

// 电流为 0 时 usbVI 不应返回有效值（避免 0W 假读数）
func TestUSBVIZeroCurrent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "voltage_now"), []byte("5200000\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "current_now"), []byte("0\n"), 0o644)
	if _, ok := usbVI(dir, true); ok {
		t.Fatal("电流为 0 时不应返回有效功率")
	}
}

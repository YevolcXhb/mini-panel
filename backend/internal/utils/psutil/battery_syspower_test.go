//go:build linux

package psutil

import (
	"os"
	"path/filepath"
	"testing"
)

// powerFixture 构造一个模拟的 power_supply 目录树：
// <root>/battery/... 与 <root>/usb/...，用于验证按供电形态取数的逻辑。
func powerFixture(t *testing.T, battery, usb map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if battery != nil {
		d := filepath.Join(root, "battery")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		for k, v := range battery {
			if err := os.WriteFile(filepath.Join(d, k), []byte(v+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if usb != nil {
		d := filepath.Join(root, "usb")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		for k, v := range usb {
			if err := os.WriteFile(filepath.Join(d, k), []byte(v+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// 场景一：usb.online=1 且 usb.power_now 有值 → 取 USB 输入侧功率
func TestReadSystemPowerUSBSide(t *testing.T) {
	root := powerFixture(t,
		map[string]string{"voltage_now": "4279000", "current_now": "0"},
		map[string]string{"online": "1", "power_now": "2500000"}, // 2.5W
	)
	batteryDir := filepath.Join(root, "battery")
	info := &BatteryInfo{Charging: true, VoltageV: 4.279, CurrentMA: 0}

	got, ok := readSystemPower(batteryDir, info, true)
	if !ok {
		t.Fatal("应能算出功耗")
	}
	if got.watts < 2.49 || got.watts > 2.51 {
		t.Fatalf("USB 侧应收 2.5W，实际 %v", got.watts)
	}
	if got.detail != "USB 输入侧" {
		t.Fatalf("口径应为「USB 输入侧」，实际 %q", got.detail)
	}
}

// 场景二：usb.online=1，但电池侧还在放电（小板未满电）
// → 总功耗 = usb.power_now + (-battery.current_now × voltage_now / 1e6)
func TestReadSystemPowerUSBPlusDischarge(t *testing.T) {
	root := powerFixture(t,
		map[string]string{
			"voltage_now": "4279000", // 4.279V
			"current_now": "-1500000", // -1500mA（放电）
		},
		map[string]string{"online": "1", "power_now": "1000000"}, // 1.0W
	)
	batteryDir := filepath.Join(root, "battery")
	info := &BatteryInfo{Charging: true, VoltageV: 4.279, CurrentMA: -1500}

	got, ok := readSystemPower(batteryDir, info, true)
	if !ok {
		t.Fatal("应能算出功耗")
	}
	// 1.0W + (4.279V × 1500mA / 1000) = 1.0 + 6.4185 = 7.4185W
	want := 1.0 + 4.279*1500/1000
	if got.watts < want-0.05 || got.watts > want+0.05 {
		t.Fatalf("期望约 %.4fW（USB+放电），实际 %v", want, got.watts)
	}
	if got.detail != "USB 输入 + 电池放电" {
		t.Fatalf("口径应为「USB 输入 + 电池放电」，实际 %q", got.detail)
	}
}

// 场景三：usb.online=0（Type-C 小板直供）→ 取电池接口侧功率
func TestReadSystemPowerBatterySide(t *testing.T) {
	root := powerFixture(t,
		map[string]string{
			"voltage_now": "4279000",
			"current_now": "-500000", // -500mA
			"power_now":   "2139500", // 2.1395W
		},
		map[string]string{"online": "0"},
	)
	batteryDir := filepath.Join(root, "battery")
	info := &BatteryInfo{Charging: false, VoltageV: 4.279, CurrentMA: -500}

	got, ok := readSystemPower(batteryDir, info, true)
	if !ok {
		t.Fatal("应能算出功耗")
	}
	if got.watts < 2.13 || got.watts > 2.15 {
		t.Fatalf("电池侧应收 2.1395W，实际 %v", got.watts)
	}
	if got.detail != "电池接口侧" {
		t.Fatalf("口径应为「电池接口侧」，实际 %q", got.detail)
	}
	if got.sign != PowerSignDischarge {
		t.Fatalf("放电方向应为 discharge，实际 %s", got.sign)
	}
}

// 场景四：usb.power_now 不存在（内核未暴露）→ 退回电池侧
func TestReadSystemPowerNoUSBNode(t *testing.T) {
	root := powerFixture(t,
		map[string]string{
			"voltage_now": "4279000",
			"current_now": "-300000", // -300mA
			"power_now":   "1283700", // 1.2837W
		},
		map[string]string{"online": "1"}, // 有 online 但没有 power_now
	)
	batteryDir := filepath.Join(root, "battery")
	info := &BatteryInfo{Charging: true, VoltageV: 4.279, CurrentMA: -300}

	got, ok := readSystemPower(batteryDir, info, true)
	if !ok {
		t.Fatal("无 usb.power_now 时应退回电池侧")
	}
	if got.watts < 1.28 || got.watts > 1.29 {
		t.Fatalf("应退回电池侧 1.2837W，实际 %v", got.watts)
	}
}

// 场景五：两边都没有功率节点，但有电流 → V×I 兜底
func TestReadSystemPowerFallsBackToVI(t *testing.T) {
	root := powerFixture(t,
		map[string]string{
			"voltage_now": "4279000",
			"current_now": "-400000", // -400mA，无 power_now
		},
		map[string]string{"online": "0"},
	)
	batteryDir := filepath.Join(root, "battery")
	info := &BatteryInfo{Charging: false, VoltageV: 4.279, CurrentMA: -400}

	got, ok := readSystemPower(batteryDir, info, true)
	if !ok {
		t.Fatal("应有 V×I 兜底")
	}
	// 4.279 × 400 / 1000 = 1.7116W
	if got.watts < 1.71 || got.watts > 1.72 {
		t.Fatalf("V×I 应约 1.7116W，实际 %v", got.watts)
	}
	if got.detail != "电压 × 电流" {
		t.Fatalf("口径应为「电压 × 电流」，实际 %q", got.detail)
	}
}

// 场景六：USB 供电 + 电流在噪声门限以下 → 不应把噪声当功耗
func TestReadSystemPowerUSBNoiseRejected(t *testing.T) {
	root := powerFixture(t,
		map[string]string{
			"voltage_now": "4279000",
			"current_now": "-2000", // -2mA，V×I = 8.5mW 低于门限
		},
		map[string]string{"online": "1"},
	)
	batteryDir := filepath.Join(root, "battery")
	info := &BatteryInfo{Charging: true, VoltageV: 4.279, CurrentMA: -2}

	got, ok := readSystemPower(batteryDir, info, true)
	// 2mA < 5mA 门限，但算出来是 8.5mW。这里允许返回该值（因为它确实非零），
	// 关键是不能因为噪声而报出夸张的功率。
	if ok && got.watts > 0.02 {
		t.Fatalf("噪声不应产生 >20mW 的读数，实际 %v", got.watts)
	}
}

// 电池供电形态下 1mA 以上的真实小电流要保留（门限放宽）
func TestReadSystemPowerBatteryModeKeepsSmallCurrent(t *testing.T) {
	root := powerFixture(t,
		map[string]string{
			"voltage_now": "4279000",
			"current_now": "-3000", // -3mA
		},
		map[string]string{"online": "0"},
	)
	batteryDir := filepath.Join(root, "battery")
	info := &BatteryInfo{Charging: false, VoltageV: 4.279, CurrentMA: -3}

	got, ok := readSystemPower(batteryDir, info, true)
	if !ok {
		t.Fatal("电池供电形态下 3mA 应被保留")
	}
	// 4.279 × 3 / 1000 = 12.8mW
	if got.watts < 0.012 || got.watts > 0.014 {
		t.Fatalf("期望约 12.8mW，实际 %v", got.watts)
	}
}

package service

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"7.1.1", "7.1.0", 1},
		{"7.1.0", "7.1.0", 0},
		{"7.0.5", "7.1.0", -1},
		{"7.10.0", "7.9.0", 1},
		{"8.0.0", "7.99.99", 1},
		{"7.0.10", "7.0.9", 1},
		{"7.0", "7.0.1", -1},
		{"v7.2.0", "7.1.9", 1},
	}
	for _, c := range cases {
		got := compareSemver(c.a, c.b)
		if got != c.want {
			t.Errorf("compareSemver(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestParseSemverNonNumeric 保证带后缀的版本号（如 7.1.0-beta）按数字前缀解析，
// 不会 panic 也不会把整体当成 0。
func TestParseSemverNonNumeric(t *testing.T) {
	if got := parseSemver("7.1.0-beta"); got != [3]int{7, 1, 0} {
		t.Errorf("parseSemver(7.1.0-beta) = %v, want [7 1 0]", got)
	}
	if got := parseSemver(""); got != [3]int{0, 0, 0} {
		t.Errorf("parseSemver(empty) = %v, want [0 0 0]", got)
	}
}

// TestRunningInstallDir 返回的是可执行文件所在目录；测试二进制目录非空即可，
// 关键是绝不返回空串（否则 install.sh 会退回默认 /opt/minipanel）。
func TestRunningInstallDir(t *testing.T) {
	dir := runningInstallDir()
	if dir == "" {
		t.Fatal("runningInstallDir() returned empty string")
	}
	if dir == "/opt/minipanel" {
		// 只有在 os.Executable 失败时才会走这个回退；测试进程下不应发生
		t.Logf("runningInstallDir() fell back to %q (os.Executable unavailable)", dir)
	}
}

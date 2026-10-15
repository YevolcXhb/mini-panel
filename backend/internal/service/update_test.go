package service

import (
	"strings"
	"testing"
)

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

// TestParseSemverNonNumeric ensures suffixed versions (e.g. 7.1.0-beta) parse by numeric prefix; no panic, not all-zero.
func TestParseSemverNonNumeric(t *testing.T) {
	if got := parseSemver("7.1.0-beta"); got != [3]int{7, 1, 0} {
		t.Errorf("parseSemver(7.1.0-beta) = %v, want [7 1 0]", got)
	}
	if got := parseSemver(""); got != [3]int{0, 0, 0} {
		t.Errorf("parseSemver(empty) = %v, want [0 0 0]", got)
	}
}

// TestRunningInstallDir returns the executable's directory; must never be empty, or install.sh would fall back to /opt/minipanel.
func TestRunningInstallDir(t *testing.T) {
	dir := runningInstallDir()
	if dir == "" {
		t.Fatal("runningInstallDir() returned empty string")
	}
	if dir == "/opt/minipanel" {
		// only reached when os.Executable fails; should not happen in tests
		t.Logf("runningInstallDir() fell back to %q (os.Executable unavailable)", dir)
	}
}

// TestRawURLCandidates 锁定候选 URL 的结构：官方 raw 直连必须在首位，
// 后面依次是 install.sh download_with_fallback 同款的三个国内镜像。
// 顺序是行为契约——首位直连保证海外/正常网络零镜像依赖，镜像只做兜底。
func TestRawURLCandidates(t *testing.T) {
	got := rawURLCandidates("install.sh")
	wantPrefixes := []string{
		"https://raw.githubusercontent.com/",
		"https://gh-proxy.com/",
		"https://ghproxy.cn/",
		"https://ghps.cc/",
	}
	if len(got) != len(wantPrefixes) {
		t.Fatalf("got %d candidates, want %d: %v", len(got), len(wantPrefixes), got)
	}
	for i, prefix := range wantPrefixes {
		if !strings.HasPrefix(got[i], prefix) {
			t.Errorf("candidate[%d] = %q, want prefix %q", i, got[i], prefix)
		}
	}
	if !strings.HasSuffix(got[0], "/YevolcXhb/mini-panel/main/install.sh") {
		t.Errorf("official URL wrong: %q", got[0])
	}
	// 镜像 URL 必须是 "https://mirror/" + 完整官方 URL 的拼接格式
	for _, i := range []int{1, 2, 3} {
		if !strings.HasPrefix(got[i], wantPrefixes[i]+wantPrefixes[0]) {
			t.Errorf("mirror candidate[%d] should embed full official URL: %q", i, got[i])
		}
	}
}

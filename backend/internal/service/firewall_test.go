package service

import (
	"strings"
	"testing"

	"github.com/minipanel/minipanel/internal/model"
)

func TestAndroidIptablesCandidates(t *testing.T) {
	roots := []string{"/proc/123/root", "/proc/1/root"}
	candidates := androidIptablesCandidates("iptables", roots)
	if len(candidates) < 4 {
		t.Fatalf("expected at least 4 candidates, got %d: %v", len(candidates), candidates)
	}

	// 最后一个始终是 chroot 内系统命令兜底
	last := candidates[len(candidates)-1]
	if len(last) != 1 || last[0] != "iptables" {
		t.Fatalf("last candidate should be system iptables, got %v", last)
	}

	// 必须包含 magiskd 逃逸路径的直接调用与 chroot 调用
	foundDirect := false
	foundChroot := false
	for _, c := range candidates {
		if len(c) == 1 && c[0] == "/proc/123/root/system/bin/iptables" {
			foundDirect = true
		}
		if len(c) == 3 && c[0] == "chroot" && c[1] == "/proc/123/root/" && c[2] == "/system/bin/iptables" {
			foundChroot = true
		}
	}
	if !foundDirect || !foundChroot {
		t.Fatalf("missing escape candidates for magiskd root: %v", candidates)
	}
}

func TestAndroidIptablesCandidatesEmptyRoots(t *testing.T) {
	candidates := androidIptablesCandidates("ip6tables", nil)
	if len(candidates) != 1 || candidates[0][0] != "ip6tables" {
		t.Fatalf("empty roots should only keep system fallback, got %v", candidates)
	}
}

func TestBuildAndroidPersistScript(t *testing.T) {
	rules := []model.FirewallRule{
		{Name: "deny-ssh", Type: "port", Action: "deny", Protocol: "tcp", Port: "22", Direction: "in", Enabled: true},
		{Name: "allow-http", Type: "port", Action: "allow", Protocol: "tcp", Port: "80,443", Direction: "in", Enabled: true},
		{Name: "block-ip", Type: "ip", Action: "deny", IP: "10.0.0.8", Direction: "in", Enabled: true},
		{Name: "out-rule", Type: "port", Action: "deny", Protocol: "all", Port: "8080", Direction: "out", Enabled: true},
		{Name: "disabled", Type: "port", Action: "deny", Protocol: "tcp", Port: "23", Enabled: false},
	}
	script := buildAndroidPersistScript(rules)
	for _, want := range []string{
		"#!/system/bin/sh",
		"sleep 30",
		"AIPT=/system/bin/iptables",
		"$AIPT -C INPUT -p tcp --dport 22 -j DROP 2>/dev/null || $AIPT -A INPUT -p tcp --dport 22 -j DROP",
		"$AIPT -C INPUT -p tcp --dport 80 -j ACCEPT 2>/dev/null || $AIPT -A INPUT -p tcp --dport 80 -j ACCEPT",
		"$AIPT -C INPUT -p tcp --dport 443 -j ACCEPT 2>/dev/null || $AIPT -A INPUT -p tcp --dport 443 -j ACCEPT",
		"$AIPT -C INPUT -s 10.0.0.8 -j DROP 2>/dev/null || $AIPT -A INPUT -s 10.0.0.8 -j DROP",
		"$AIPT -C OUTPUT -p tcp --dport 8080 -j DROP 2>/dev/null || $AIPT -A OUTPUT -p tcp --dport 8080 -j DROP",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("persist script missing %q:\n%s", want, script)
		}
	}
	if strings.Contains(script, "dport 23") {
		t.Errorf("disabled rule should be skipped:\n%s", script)
	}
}

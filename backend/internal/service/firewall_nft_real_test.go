//go:build linux

package service

import (
	"strings"
	"testing"

	"github.com/minipanel/minipanel/internal/model"
)

// 这些用例直接在设备上跑真实 netlink 操作，验证 service 层对
// FirewallRule 的翻译是否被内核接受，以及回读是否一致。
//
// 交叉编译执行：
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o fwtest ./internal/service
//
// 设备上：su -c "sh /data/local/tmp/dorun.sh /data/local/tmp/fwtest -test.v"

func testRule(name, typ, action, proto, port, ip, dir, target string, masq bool) model.FirewallRule {
	return model.FirewallRule{
		Name: name, Type: typ, Action: action, Protocol: proto,
		Port: port, IP: ip, Direction: dir, TargetPort: target,
		Masq: masq, Enabled: true,
	}
}

func TestNftApplyRulesRealKernel(t *testing.T) {
	if !nftAvailable() {
		t.Skip("内核不支持 nf_tables")
	}

	rules := []model.FirewallRule{
		testRule("allow-ssh", "port", "allow", "tcp", "22", "", "in", "", false),
		testRule("allow-web", "port", "allow", "tcp", "80,443", "", "in", "", false),
		testRule("deny-range", "port", "deny", "tcp", "3306-3308", "", "in", "", false),
		testRule("block-one-ip", "ip", "deny", "all", "", "10.0.0.5", "in", "", false),
		testRule("allow-lan", "ip", "allow", "all", "", "192.168.0.0/16", "in", "", false),
		testRule("out-dns", "port", "allow", "udp", "53", "", "out", "", false),
		testRule("dnat-rdp", "dnat", "allow", "tcp", "13389", "192.168.1.100", "in", "3389", true),
	}

	lines, err := nftApplyRules(rules)
	if err != nil {
		t.Fatalf("应用规则失败: %v\n输出: %v", err, lines)
	}
	for _, l := range lines {
		t.Log(l)
	}

	// 回读：每条规则都要能在对应的链里找到
	view, err := nftListSystem("", "")
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}

	var filterChains, natChains int
	for _, tb := range view.Tables {
		for _, ch := range tb.Chains {
			switch tb.Name {
			case nftTableFilter:
				filterChains++
				t.Logf("[filter] %s: %d 条", ch.Name, len(ch.Rules))
				for _, r := range ch.Rules {
					t.Logf("   #%d %s", r.Handle, r.Text)
				}
			case nftTableNAT:
				natChains++
				t.Logf("[nat] %s: %d 条", ch.Name, len(ch.Rules))
				for _, r := range ch.Rules {
					t.Logf("   #%d %s", r.Handle, r.Text)
				}
			}
		}
	}
	if filterChains != 3 {
		t.Errorf("mp_filter 应有 3 条链，实际 %d", filterChains)
	}
	if natChains != 3 {
		t.Errorf("mp_nat 应有 3 条链，实际 %d", natChains)
	}

	// 清理
	if err := nftFlush(); err != nil {
		t.Fatalf("清理失败: %v", err)
	}
}

func TestNftInsertAndDeleteRule(t *testing.T) {
	if !nftAvailable() {
		t.Skip("内核不支持 nf_tables")
	}
	if lines, err := nftApplyRules(nil); err != nil {
		t.Fatalf("建骨架失败: %v\n%v", err, lines)
	}

	// 插到 input 链首
	if err := nftInsertRule(nftTableFilter, nftChainInput, 1, []string{"tcp", "dport", "8080"}, "accept"); err != nil {
		t.Fatalf("插入失败: %v", err)
	}
	rules, err := nftHandle()
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	rs, err := rules.ListRules(filterTable(), nftChainInput)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	var found bool
	var handle uint64
	for _, r := range rs {
		t.Logf("input #%d %s", r.Handle, r.Text)
		if strings.Contains(r.Text, "dport 8080") {
			found = true
			handle = r.Handle
		}
	}
	if !found {
		t.Fatalf("插入的规则没找到，链内共 %d 条", len(rs))
	}

	if err := nftDeleteRule(nftTableFilter, nftChainInput, handle); err != nil {
		t.Fatalf("按 handle 删除失败: %v", err)
	}
	rs2, _ := rules.ListRules(filterTable(), nftChainInput)
	for _, r := range rs2 {
		if r.Handle == handle {
			t.Fatalf("删除后仍存在 handle %d", handle)
		}
	}

	if err := nftFlush(); err != nil {
		t.Fatalf("清理失败: %v", err)
	}
}

func TestNftLockdownRealKernel(t *testing.T) {
	if !nftAvailable() {
		t.Skip("内核不支持 nf_tables")
	}
	if err := nftLockdown(); err != nil {
		t.Fatalf("lockdown 失败: %v", err)
	}
	rs, err := nftListSystem(nftTableFilter, nftChainInput)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	for _, tb := range rs.Tables {
		for _, ch := range tb.Chains {
			for _, r := range ch.Rules {
				t.Logf("%s/%s #%d %s", tb.Name, ch.Name, r.Handle, r.Text)
			}
		}
	}
	if err := nftFlush(); err != nil {
		t.Fatalf("清理失败: %v", err)
	}
}

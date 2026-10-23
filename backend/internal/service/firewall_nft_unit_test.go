//go:build linux

package service

import (
	"net"
	"testing"

	"github.com/minipanel/minipanel/internal/nft"
)

// TestCidrRange 校验网段首尾地址计算。
// 曾经的 bug：右移量写成 bits-ones，/8 算成 10.0.0.0-10.0.0.255、
// /12 算成 172.16.0.0-172.16.15.255，导致内网段放行范围严重偏小。
func TestCidrRange(t *testing.T) {
	cases := []struct {
		cidr     string
		lo, hi   string
	}{
		{"192.168.0.0/16", "192.168.0.0", "192.168.255.255"},
		{"10.0.0.0/8", "10.0.0.0", "10.255.255.255"},
		{"172.16.0.0/12", "172.16.0.0", "172.31.255.255"},
		{"192.168.1.0/24", "192.168.1.0", "192.168.1.255"},
		{"0.0.0.0/0", "0.0.0.0", "255.255.255.255"},
		{"203.0.113.7/32", "203.0.113.7", "203.0.113.7"},
	}
	for _, cs := range cases {
		_, netw, err := net.ParseCIDR(cs.cidr)
		if err != nil {
			t.Fatalf("%s 解析失败: %v", cs.cidr, err)
		}
		lo, hi, ok := cidrRange(netw)
		if !ok {
			t.Fatalf("%s 未返回结果", cs.cidr)
		}
		wantLo := beUint32(net.ParseIP(cs.lo).To4())
		wantHi := beUint32(net.ParseIP(cs.hi).To4())
		if lo != wantLo || hi != wantHi {
			t.Errorf("%s 期望 %s-%s，得到 %s-%s",
				cs.cidr, cs.lo, cs.hi, ipStr(lo), ipStr(hi))
		}
	}
}

func ipStr(v uint32) string {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).String()
}

// TestMasqExpressionName 校验 masquerade 用的表达式名。
// 内核注册的名字是 "masq"；写成 "masquerade" 会 ENOENT（not found），
// 而写成 nat 表达式的 type=2 变体会 EINVAL。两个坑都踩过。
func TestMasqExpressionName(t *testing.T) {
	if nft.ExprMasq != "masq" {
		t.Fatalf("ExprMasq 应为 masq，实际 %q", nft.ExprMasq)
	}
	if !nftAvailable() {
		t.Skip("内核不支持 nf_tables")
	}
	c, err := nftHandle()
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	ft, nt := filterTable(), natTable()
	cleanup := func() {
		for _, tb := range []nft.Table{ft, nt} {
			if ok, _ := c.HasTable(tb); ok {
				_ = c.Apply([]nft.BatchOp{nft.DelTableOp(tb)})
			}
		}
	}
	cleanup()
	defer cleanup()

	if err := c.Apply(nftBaseOps()); err != nil {
		t.Fatalf("建骨架失败: %v", err)
	}
	op := nft.AddRuleOp(nft.Rule{Table: nt, Chain: nftChainPostRoute, Exprs: []nft.Expr{nft.Masq()}})
	if err := c.Apply([]nft.BatchOp{op}); err != nil {
		t.Fatalf("masq 规则被内核拒绝: %v", err)
	}
	rs, err := c.ListRules(nt, nftChainPostRoute)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("postrouting 应有 1 条规则，实际 %d", len(rs))
	}
}

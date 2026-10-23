//go:build linux

package service

import (
	"testing"

	"github.com/minipanel/minipanel/internal/nft"
)

// TestNftBatchPathDebug 逐步验证 batch.go 的 c.Apply 路径：
// 单条建表 → 多条建表 → 建表+建链，每步都回读确认是否真的落到了内核。
func TestNftBatchPathDebug(t *testing.T) {
	if !nftAvailable() {
		t.Skip("内核不支持 nf_tables")
	}
	c, err := nftHandle()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	clean := func() {
		_ = c.Apply([]nft.BatchOp{
			nft.DelTableOp(filterTable()),
			nft.DelTableOp(natTable()),
		})
	}
	clean()
	defer clean()

	exists := func(tb nft.Table) bool {
		ok, _ := c.HasTable(tb)
		return ok
	}

	// 1) 单条建表（走 batch）
	t.Log("--- 1) Apply 单条建表 mp_filter")
	err = c.Apply([]nft.BatchOp{nft.NewTableOp(filterTable())})
	t.Logf("err=%v exists=%v", err, exists(filterTable()))
	if !exists(filterTable()) {
		t.Fatal("单条建表未生效")
	}

	// 2) 再建一张
	t.Log("--- 2) Apply 建 mp_nat")
	err = c.Apply([]nft.BatchOp{nft.NewTableOp(natTable())})
	t.Logf("err=%v exists=%v", err, exists(natTable()))

	// 3) 建链（单独一条）
	t.Log("--- 3) Apply 建 input 链")
	err = c.Apply([]nft.BatchOp{nft.NewChainOp(nft.Chain{
		Table: filterTable(), Name: nftChainInput, Type: nft.ChainTypeFilter,
		Hook: nft.HookInput, Priority: 0, Policy: nft.VerdictAccept,
	})})
	chains, cerr := c.ListChains(filterTable())
	t.Logf("err=%v chains=%d cerr=%v", err, len(chains), cerr)

	// 4) 多条混合：清空重建的完整骨架
	clean()
	t.Log("--- 4) Apply nftBaseOps（6 条）")
	err = c.Apply(nftBaseOps())
	t.Logf("err=%v filter=%v nat=%v", err, exists(filterTable()), exists(natTable()))
	ch1, _ := c.ListChains(filterTable())
	ch2, _ := c.ListChains(natTable())
	t.Logf("chains: filter=%d nat=%d", len(ch1), len(ch2))

	// 5) 在已有骨架的批次里加一条规则
	if len(ch1) > 0 {
		t.Log("--- 5) Apply 一条规则到 input")
		err = c.Apply([]nft.BatchOp{nft.AddRuleOp(nft.Rule{
			Table: filterTable(), Chain: nftChainInput,
			Exprs: []nft.Expr{nft.Counter(), nft.ImmediateVerdict(nft.VerdictAccept)},
		})})
		rs, rerr := c.ListRules(filterTable(), nftChainInput)
		t.Logf("err=%v rules=%d rerr=%v", err, len(rs), rerr)
		for _, r := range rs {
			t.Logf("   #%d %s", r.Handle, r.Text)
		}
	}
}

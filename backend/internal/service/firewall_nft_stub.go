//go:build !linux

package service

// 非 Linux 平台没有 nf_tables，这里提供同名桩函数，
// 让 firewall.go 里的分支代码无需平台条件编译即可通过编译。
// 这些函数在非 Linux 上永远只会被 nftAvailable()==false 拦住，实际不会被调用。
// 表名/链名常量与对外数据结构在 firewall_nft_names.go（平台无关）。

import (
	"fmt"
	"sync"

	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/nft"
)

var (
	nftMu   sync.Mutex
	nftConn *nft.Conn
)

func nftAvailable() bool { return false }

func nftHandle() (*nft.Conn, error) {
	return nil, fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

func closeNft() {}

func filterTable() nft.Table { return nft.Table{Family: nft.FamilyIPv4, Name: nftTableFilter} }
func natTable() nft.Table    { return nft.Table{Family: nft.FamilyIPv4, Name: nftTableNAT} }

func nftEnsureBase() error { return fmt.Errorf("nf_tables 仅在 Linux 上可用") }
func nftFlush() error      { return fmt.Errorf("nf_tables 仅在 Linux 上可用") }

func nftApplyRules(rules []model.FirewallRule) ([]string, error) {
	return nil, fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

func nftListSystem(tableName, chainName string) (*NftTableView, error) {
	return nil, fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

func nftDeleteRule(tableName, chainName string, handle uint64) error {
	return fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

func nftInsertRule(tableName, chainName string, position uint64, spec []string, action string) error {
	return fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

func nftLockdown() error { return fmt.Errorf("nf_tables 仅在 Linux 上可用") }

func (s *FirewallService) SystemRules(tableName, chainName string) (*NftTableView, error) {
	return nil, fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

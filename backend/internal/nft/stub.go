//go:build !linux

// 非 Linux 平台上的最小桩实现。nf_tables 是 Linux 内核特性，
// 这里只提供类型和常量，让上层代码能编译；任何实际调用都返回错误。

package nft

import "fmt"

const (
	FamilyUnspec uint8 = 0
	FamilyInet   uint8 = 1
	FamilyIPv4   uint8 = 2
	FamilyIPv6   uint8 = 10
)

// Table 标识一张 nft 表
type Table struct {
	Family uint8
	Name   string
}

// Conn 是 netlink 连接的桩
type Conn struct{}

// Dial 在非 Linux 上永远失败
func Dial() (*Conn, error) {
	return nil, fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

// Close 关闭连接
func (c *Conn) Close() error { return nil }

// Fd 返回底层文件描述符
func (c *Conn) Fd() int { return -1 }

// HasTable 检查表是否存在
func (c *Conn) HasTable(t Table) (bool, error) {
	return false, fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

// RuleInfo 是内核里一条规则的读法（桩）
type RuleInfo struct {
	Handle   uint64
	Chain    string
	Text     string
	Verdict  string
	Packets  uint64
	Bytes    uint64
	HasCount bool
}

// ListRules 列举某条链上的规则（桩）
func (c *Conn) ListRules(t Table, chain string) ([]RuleInfo, error) {
	return nil, fmt.Errorf("nf_tables 仅在 Linux 上可用")
}

// Available 报告 nf_tables 是否可用
func Available() bool { return false }

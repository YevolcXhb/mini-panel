//go:build linux

package nft

import "syscall"

// BatchOp 描述一次规则集变更里的单步操作。
//
// 一批操作在**同一个 netlink batch** 里提交，内核以单次 commit 生效：
// 要么全部生效，要么全部不生效。重写规则集就靠这个特性做到
// 「清空 + 重建」对外表现为原子替换，中间不存在放行窗口。
type BatchOp struct {
	Kind  BatchKind
	Table Table
	Chain Chain
	Rule  Rule
}

// BatchKind 是 BatchOp 的种类
type BatchKind int

const (
	OpNewTable BatchKind = iota
	OpDelTable
	OpNewChain
	OpDelChain
	// OpFlushChain 清空链上所有规则
	OpFlushChain
	// OpAddRule 追加规则
	OpAddRule
)

// NewTableOp / NewChainOp / ... 是构造 BatchOp 的便捷函数
func NewTableOp(t Table) BatchOp             { return BatchOp{Kind: OpNewTable, Table: t} }
func DelTableOp(t Table) BatchOp             { return BatchOp{Kind: OpDelTable, Table: t} }
func NewChainOp(ch Chain) BatchOp            { return BatchOp{Kind: OpNewChain, Chain: ch} }
func DelChainOp(ch Chain) BatchOp            { return BatchOp{Kind: OpDelChain, Chain: ch} }
func FlushChainOp(t Table, chain string) BatchOp {
	return BatchOp{Kind: OpFlushChain, Table: t, Chain: Chain{Table: t, Name: chain}}
}
func AddRuleOp(r Rule) BatchOp { return BatchOp{Kind: OpAddRule, Rule: r} }

// toMsg 把单步操作转成待发消息
func (op BatchOp) toMsg(c *Conn) (msg, bool) {
	switch op.Kind {
	case OpNewTable:
		var a attrBuf
		a.Str(AttrTableName, op.Table.Name)
		return msg{
			msgType: nftType(MsgNewTable),
			flags:   syscall.NLM_F_REQUEST | syscall.NLM_F_CREATE | syscall.NLM_F_EXCL,
			body:    append(nfgen(op.Table.Family, 0), a.b...),
		}, true

	case OpDelTable:
		var a attrBuf
		a.Str(AttrTableName, op.Table.Name)
		return msg{
			msgType: nftType(MsgDelTable),
			flags:   syscall.NLM_F_REQUEST,
			body:    append(nfgen(op.Table.Family, 0), a.b...),
		}, true

	case OpNewChain:
		return msg{
			msgType: nftType(MsgNewChain),
			flags:   syscall.NLM_F_REQUEST | syscall.NLM_F_CREATE | syscall.NLM_F_EXCL,
			body:    c.chainBody(op.Chain),
		}, true

	case OpDelChain:
		var a attrBuf
		a.Str(AttrChainTable, op.Chain.Table.Name)
		a.Str(AttrChainName, op.Chain.Name)
		return msg{
			msgType: nftType(MsgDelChain),
			flags:   syscall.NLM_F_REQUEST,
			body:    append(nfgen(op.Chain.Table.Family, 0), a.b...),
		}, true

	case OpFlushChain:
		return msg{
			msgType: nftType(MsgDelRule),
			flags:   syscall.NLM_F_REQUEST,
			body:    chainBodyWithName(op.Chain.Table, op.Chain.Name),
		}, true

	case OpAddRule:
		return msg{
			msgType: nftType(MsgNewRule),
			// 必须带 NLM_F_APPEND：不带时内核把新规则插到**链头**（prepend），
			// 一批里按自然顺序逐条添加后链内顺序完全颠倒，末尾的兜底 drop
			// 会跑到最前面把流量全吃掉（真机实测，见 lockprobe5 v13/v14）。
			flags: syscall.NLM_F_REQUEST | syscall.NLM_F_CREATE | syscall.NLM_F_APPEND,
			body:  c.ruleBody(op.Rule),
		}, true
	}
	return msg{}, false
}

// Apply 把一批操作作为单个事务提交。
//
// 与逐条调用 NewRule 的区别：逐条提交时中途失败会留下半套规则；
// 这里整批一次 commit，失败则内核回滚，规则集保持原样。
func (c *Conn) Apply(ops []BatchOp) error {
	if len(ops) == 0 {
		return nil
	}
	msgs := make([]msg, 0, len(ops))
	for _, op := range ops {
		m, ok := op.toMsg(c)
		if !ok {
			continue
		}
		msgs = append(msgs, m)
	}
	if len(msgs) == 0 {
		return nil
	}
	return c.runBatch(msgs)
}

// Available 探测内核 nf_tables 是否可用（能 dial 且能 dump 出表列表）。
//
// 用于在没有 nft 用户态程序的精简系统上判断能否走 netlink 直连。
func Available() bool {
	c, err := Dial()
	if err != nil {
		return false
	}
	defer c.Close()
	_, err = c.ListTables(FamilyIPv4)
	return err == nil
}

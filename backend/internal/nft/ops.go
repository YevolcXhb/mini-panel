//go:build linux

package nft

import (
	"fmt"
	"syscall"
)

// ─── 表 ───

// Table 描述一张 nft 表
type Table struct {
	Family uint8
	Name   string
}

func nftType(msgType uint16) uint16 {
	return (NFNL_SUBSYS_NFTABLES << 8) | msgType
}

// NewTable 建表。表已存在返回 ErrExists。
//
// 必须带 NLM_F_EXCL：不带的话对已存在的表建表是静默无操作（返回 0），
// 这样就没法用「试建」来探测存在性。
func (c *Conn) NewTable(t Table) error {
	var a attrBuf
	a.Str(AttrTableName, t.Name)
	body := append(nfgen(t.Family, 0), a.b...)

	return c.runBatch([]msg{{
		msgType: nftType(MsgNewTable),
		flags:   syscall.NLM_F_REQUEST | syscall.NLM_F_CREATE | syscall.NLM_F_EXCL,
		body:    body,
	}})
}

// DelTable 删表。表不存在返回 ErrNoEnt。
func (c *Conn) DelTable(t Table) error {
	var a attrBuf
	a.Str(AttrTableName, t.Name)
	body := append(nfgen(t.Family, 0), a.b...)

	return c.runBatch([]msg{{
		msgType: nftType(MsgDelTable),
		flags:   syscall.NLM_F_REQUEST,
		body:    body,
	}})
}

// HasTable 通过「尝试建同名表」判断表是否存在。
//
// 本内核的 GETTABLE dump 回调不回包（实测 NO REPLY），所以不能靠读。
// 建同名表得到 EEXIST 就说明表在；得到 nil 说明表是刚建的（说明之前不存在）。
// 为避免副作用，若表不存在会立刻删掉刚建的这张空表。
func (c *Conn) HasTable(t Table) (bool, error) {
	err := c.NewTable(t)
	switch {
	case err == ErrExists:
		return true, nil
	case err == nil:
		// 刚建成功 → 原本不存在，回滚
		_ = c.DelTable(t)
		return false, nil
	default:
		return false, err
	}
}

// ─── 链 ───

// Chain 描述一条链
type Chain struct {
	Table    Table
	Name     string
	Type     string // filter / route / nat；普通链留空
	Hook     int    // 仅 base chain 需要
	Priority int    // 仅 base chain 需要
	Policy   int    // 内置链策略：VerdictAccept / VerdictDrop；PolicyUnset=不设置
}

// PolicyUnset 表示不指定链策略。
// 不能用 0 表示"不设置"，因为 NF_DROP 本身就是 0。
const PolicyUnset = -256

func (c *Conn) chainBody(ch Chain) []byte {
	var a attrBuf
	a.Str(AttrChainTable, ch.Table.Name)
	a.Str(AttrChainName, ch.Name)

	if ch.Type != "" {
		a.Str(AttrChainType, ch.Type)
		// NFTA_CHAIN_HOOK 是嵌套属性，内部字段用 NFTA_HOOK_* 自己的编号
		// （HOOKNUM=1、PRIORITY=2），不能复用外层的 NFTA_CHAIN_HOOK=4。
		a.Nested(AttrChainHook, func(dst *attrBuf) {
			dst.U32(AttrHookHooknum, uint32(ch.Hook))
			dst.U32(AttrHookPriority, uint32(int32(ch.Priority)))
		})
		if ch.Policy != PolicyUnset {
			a.U32(AttrChainPolicy, uint32(int32(ch.Policy)))
		}
	}
	return append(nfgen(ch.Table.Family, 0), a.b...)
}

// NewChain 建链。链已存在返回 ErrExists。
func (c *Conn) NewChain(ch Chain) error {
	return c.runBatch([]msg{{
		msgType: nftType(MsgNewChain),
		flags:   syscall.NLM_F_REQUEST | syscall.NLM_F_CREATE | syscall.NLM_F_EXCL,
		body:    c.chainBody(ch),
	}})
}

// DelChain 删链。
func (c *Conn) DelChain(ch Chain) error {
	var a attrBuf
	a.Str(AttrChainTable, ch.Table.Name)
	a.Str(AttrChainName, ch.Name)
	body := append(nfgen(ch.Table.Family, 0), a.b...)
	return c.runBatch([]msg{{
		msgType: nftType(MsgDelChain),
		flags:   syscall.NLM_F_REQUEST,
		body:    body,
	}})
}

// ─── 规则 ───

// Expr 是一条表达式
type Expr struct {
	Name string
	Data []byte // 已序列化的属性段
}

// Rule 描述一条规则
type Rule struct {
	Table Table
	Chain string
	Exprs []Expr
	// Position 为插入位置（handle）。0 表示追加到链尾。
	Position uint64
}

func (c *Conn) ruleBody(r Rule) []byte {
	var a attrBuf
	a.Str(AttrRuleTable, r.Table.Name)
	a.Str(AttrRuleChain, r.Chain)
	// NFTA_RULE_POSITION 是插入位置：不带时追加到链尾，
	// 带上则插到指定 handle 之前。
	if r.Position > 0 {
		a.U64(AttrRulePosition, r.Position)
	}
	// NFTA_RULE_EXPRESSIONS 的每个子元素都是 NFTA_LIST_ELEM(=1)，
	// 元素内部才是 { NFTA_EXPR_NAME, NFTA_EXPR_DATA }。
	a.Nested(AttrRuleExpressions, func(dst *attrBuf) {
		for _, e := range r.Exprs {
			dst.Nested(AttrListElem, func(el *attrBuf) {
				el.Str(AttrExprName, e.Name)
				if len(e.Data) > 0 {
					el.Nested(AttrExprData, func(d *attrBuf) {
						d.b = append(d.b, e.Data...)
					})
				}
			})
		}
	})
	return append(nfgen(r.Table.Family, 0), a.b...)
}

// NewRule 添加规则。
//
// 带 NLM_F_APPEND 追加到链尾；不带时内核插到链头（prepend），
// 连续添加会让规则顺序颠倒（真机实测）。
func (c *Conn) NewRule(r Rule) error {
	return c.runBatch([]msg{{
		msgType: nftType(MsgNewRule),
		flags:   syscall.NLM_F_REQUEST | syscall.NLM_F_CREATE | syscall.NLM_F_APPEND,
		body:    c.ruleBody(r),
	}})
}

// DelRule 删除整条链上的规则（按链清空）。
// 精确删单条需要 handle，本内核不回 dump 拿不到 handle，
// 因此采用「删表重建」或在链级别 flush 的方式，见 FlushChain。
func (c *Conn) FlushChain(t Table, chain string) error {
	return c.runBatch([]msg{{
		msgType: nftType(MsgDelRule),
		flags:   syscall.NLM_F_REQUEST,
		body:    chainBodyWithName(t, chain),
	}})
}

// DelRuleByHandle 按 handle 精确删除单条规则。
func (c *Conn) DelRuleByHandle(t Table, chain string, handle uint64) error {
	var a attrBuf
	a.Str(AttrRuleTable, t.Name)
	a.Str(AttrRuleChain, chain)
	a.U64(AttrRuleHandle, handle)
	body := append(nfgen(t.Family, 0), a.b...)

	return c.runBatch([]msg{{
		msgType: nftType(MsgDelRule),
		flags:   syscall.NLM_F_REQUEST,
		body:    body,
	}})
}

func chainBodyWithName(t Table, chain string) []byte {
	var a attrBuf
	a.Str(AttrRuleTable, t.Name)
	a.Str(AttrRuleChain, chain)
	return append(nfgen(t.Family, 0), a.b...)
}

// ─── 表达式构造辅助 ───

// Counter 计数器表达式
func Counter() Expr {
	var a attrBuf
	return Expr{Name: ExprCounter, Data: a.b}
}

// ImmediateVerdict 立即判决（accept / drop / continue）
//
// NFTA_IMMEDIATE_DATA 是嵌套属性，里面再挂一层 NFTA_DATA_VERDICT，
// 判决码本身还要再嵌一层 NFTA_VERDICT_CODE。漏掉这层嵌套内核会 EINVAL。
func ImmediateVerdict(verdict int32) Expr {
	var data attrBuf
	data.Nested(AttrDataVerdict, func(v *attrBuf) {
		v.U32(AttrVerdictCode, uint32(verdict))
	})

	var a attrBuf
	a.U32(AttrImmDreg, RegVerdict)
	a.Nested(AttrImmData, func(dst *attrBuf) {
		dst.b = append(dst.b, data.b...)
	})
	return Expr{Name: ExprImmediate, Data: a.b}
}

// Payload 读取报文某字段到寄存器
func Payload(base uint32, offset, length uint32, reg uint32) Expr {
	var a attrBuf
	a.U32(AttrPayloadDreg, reg)
	a.U32(AttrPayloadBase, base)
	a.U32(AttrPayloadOffset, offset)
	a.U32(AttrPayloadLen, length)
	return Expr{Name: ExprPayload, Data: a.b}
}

// Cmp 寄存器与常量比较
func Cmp(reg uint32, op uint32, value []byte) Expr {
	var data attrBuf
	data.Raw(AttrDataValue, value)

	var a attrBuf
	a.U32(AttrCmpSreg, reg)
	a.U32(AttrCmpOp, op)
	a.Nested(AttrCmpData, func(dst *attrBuf) {
		dst.b = append(dst.b, data.b...)
	})
	return Expr{Name: ExprCmp, Data: a.b}
}

// CmpU16 便捷：比较 16 位端口
func CmpU16(reg uint32, op uint32, v uint16) Expr {
	return Cmp(reg, op, []byte{byte(v >> 8), byte(v)})
}

// CmpByte 便捷：比较单字节值（如 L4 协议号）。
// 注意 meta l4proto 只有 1 字节宽，必须用 1 字节比较；
// 用 4 字节去比会被内核按整寄存器长度裁剪而永不命中。
func CmpByte(reg uint32, op uint32, v uint8) Expr {
	return Cmp(reg, op, []byte{v})
}

// CmpU32 便捷：比较 32 位 IP
func CmpU32(reg uint32, op uint32, v uint32) Expr {
	return Cmp(reg, op, []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// MetaL4Proto 取 L4 协议号到寄存器
func MetaL4Proto(reg uint32) Expr {
	var a attrBuf
	a.U32(AttrMetaKey, 16) // NFT_META_L4PROTO
	a.U32(AttrMetaDreg, reg)
	return Expr{Name: ExprMeta, Data: a.b}
}

// MetaMark 取 connmark / mark
func MetaMark(reg uint32) Expr {
	var a attrBuf
	a.U32(AttrMetaKey, 3) // NFT_META_MARK
	a.U32(AttrMetaDreg, reg)
	return Expr{Name: ExprMeta, Data: a.b}
}

// NATSNAT 源地址转换
func NATSNAT(regAddr uint32, regPortMin, regPortMax uint32) Expr {
	var a attrBuf
	a.U32(AttrNatType, 0) // 0 = snat
	a.U32(AttrNatFamily, FamilyIPv4)
	a.U32(AttrNatSregAddrMin, regAddr)
	if regPortMin != 0 {
		a.U32(AttrNatSregProtoMin, regPortMin)
	}
	if regPortMax != 0 {
		a.U32(AttrNatSregProtoMax, regPortMax)
	}
	return Expr{Name: ExprNAT, Data: a.b}
}

// NATDNAT 目的地址转换
func NATDNAT(regAddr uint32, regPortMin, regPortMax uint32) Expr {
	var a attrBuf
	a.U32(AttrNatType, 1) // 1 = dnat
	a.U32(AttrNatFamily, FamilyIPv4)
	a.U32(AttrNatSregAddrMin, regAddr)
	if regPortMin != 0 {
		a.U32(AttrNatSregProtoMin, regPortMin)
	}
	if regPortMax != 0 {
		a.U32(AttrNatSregProtoMax, regPortMax)
	}
	return Expr{Name: ExprNAT, Data: a.b}
}

// Masq 伪装（SNAT 到出接口地址）
//
// 内核表达式名是 "masq"（模块 nft_masq），不是 "masquerade"。
// 属性只有可选的 NFTA_MASQ_FLAGS(1)，不填也能用；它也不是 nat 表达式的变体
// ——「type=2 塞进 nat」实测 -22 EINVAL，真机踩过两种错法。
func Masq() Expr {
	return Expr{Name: ExprMasq}
}

// Log 记录日志
func Log(prefix string, level uint32) Expr {
	var a attrBuf
	if prefix != "" {
		a.Str(AttrLogPrefix, prefix)
	}
	if level > 0 {
		a.U32(AttrLogLevel, level)
	}
	return Expr{Name: ExprLog, Data: a.b}
}

// Reject 拒绝
func Reject(rejectType uint32) Expr {
	var a attrBuf
	a.U32(AttrRejectType, rejectType)
	return Expr{Name: ExprReject, Data: a.b}
}

// Limit 限速：rate 为每秒包数（内核要求 be64）
func Limit(rate uint64) Expr {
	var a attrBuf
	a.U64(AttrLimitRate, rate)
	a.U32(AttrLimitUnit, 1) // 1 = second
	return Expr{Name: ExprLimit, Data: a.b}
}

// String 便于调试输出
func (r Rule) String() string {
	names := ""
	for i, e := range r.Exprs {
		if i > 0 {
			names += ","
		}
		names += e.Name
	}
	return fmt.Sprintf("%s/%s: [%s]", r.Table.Name, r.Chain, names)
}

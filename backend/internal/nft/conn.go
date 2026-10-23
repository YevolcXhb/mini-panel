//go:build linux

// Package nft 提供不依赖 nft 用户态程序的 nf_tables 客户端。
//
// 设计要点（全部来自真机实测，不是照抄 uapi 猜测）：
//
//  1. nlmsghdr 的所有整数字段、以及属性头 nla_len/nla_type 都是
//     **主机字节序**（这块板子是小端）；只有 nfgenmsg 的 res_id 和
//     nla 载荷里的整数才是**网络字节序**（大端）。混用会让内核解析失败：
//     属性头写成大端时 nla_len 变成 0x0a00=2560，内核直接 EINVAL ——
//     这是本项目踩得最久的一个坑。
//
//  2. 所有修改类操作必须包在 NFNL_MSG_BATCH_BEGIN / BATCH_END 之间，
//     否则返回 EINVAL。BEGIN/END 的 nfgen_res_id 必须是子系统号 10
//     （填 0 会 EOPNOTSUPP），中间的 NFT_MSG_* 消息类型才带子系统号 10。
//
//  3. 每条消息都要带 NLM_F_ACK，否则中间消息的错误会被 BATCH_END 的
//     「提交成功」掩盖（曾误判 DELTABLE 成功删除）。
//
//  4. 建表/建链必须带 NLM_F_EXCL，否则对已存在的表建表是静默无操作，
//     拿不到 EEXIST，也就无法用「试写」探测存在性。
//
//  5. 表名/链名属性必须带结尾 NUL，长度含 NUL。
package nft

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"syscall"
)

// ─── 协议常量 ───

const (
	NETLINK_NETFILTER   = 12
	NFNL_SUBSYS_NFTABLES = 10
	NFNETLINK_V0        = 0

	NFNL_MSG_BATCH_BEGIN = 16
	NFNL_MSG_BATCH_END   = 17
)

// nft 消息类型
const (
	MsgNewTable = 0
	MsgGetTable = 1
	MsgDelTable = 2
	MsgNewChain = 3
	MsgGetChain = 4
	MsgDelChain = 5
	MsgNewRule  = 6
	MsgGetRule  = 7
	MsgDelRule  = 8
	MsgNewSet   = 9
	MsgGetSet   = 10
	MsgDelSet   = 11
	MsgNewSetElem = 12
	MsgGetSetElem = 13
	MsgDelSetElem = 14
	MsgGetGen     = 16
)

// family
const (
	FamilyUnspec = 0
	FamilyInet   = 1
	FamilyIPv4   = 2
	FamilyARP    = 3
	FamilyNetdev = 5
	FamilyBridge = 7
	FamilyIPv6   = 10
)

// table 属性
const (
	AttrTableName  = 1
	AttrTableFlags = 2
	AttrTableUse   = 3
)

// chain 属性（uapi: linux/netfilter/nf_tables.h）
const (
	AttrChainTable   = 1
	AttrChainHandle  = 2
	AttrChainName    = 3
	AttrChainHook    = 4 // 嵌套属性，内部见 AttrHook*
	AttrChainPolicy  = 5
	AttrChainUse     = 6
	AttrChainType    = 7
	AttrChainCounter = 8
	AttrChainFlags   = 10
	AttrChainID      = 11
)

// NFTA_CHAIN_HOOK 嵌套属性（注意编号与外层完全无关，从 1 重新开始）
const (
	AttrHookHooknum  = 1
	AttrHookPriority = 2
	AttrHookDev      = 3
)

// rule 属性
const (
	AttrRuleTable    = 1
	AttrRuleChain    = 2
	AttrRuleHandle   = 3
	AttrRuleExpressions = 4
	AttrRuleCompat   = 5
	// AttrRulePosition 是插入位置（handle），不设置即追加到链尾
	AttrRulePosition = 6
)

// 表达式属性
const (
	AttrExprName = 1
	AttrExprData = 2
)

// NFTA_LIST_ELEM：NFTA_*_EXPRESSIONS / ELEMENTS 列表里每个元素的属性类型
const AttrListElem = 1

// 通用表达式属性（nft_data_attributes）
const (
	AttrDataValue   = 1
	AttrDataVerdict = 2
)

// NFTA_DATA_VERDICT 嵌套属性（nft_verdict_attributes）
const (
	AttrVerdictCode  = 1
	AttrVerdictChain = 2
)

// counter 表达式
const (
	AttrCounterBytes   = 1
	AttrCounterPackets = 2
)

// immediate 表达式
const (
	AttrImmDreg = 1
	AttrImmData = 2
)

// payload 表达式
const (
	AttrPayloadDreg   = 1
	AttrPayloadBase   = 2
	AttrPayloadOffset = 3
	AttrPayloadLen    = 4
)

// cmp 表达式
const (
	AttrCmpSreg = 1
	AttrCmpOp   = 2
	AttrCmpData = 3
)

// meta 表达式（注意：DREG 是 1、KEY 是 2，与直觉相反）
const (
	AttrMetaDreg = 1
	AttrMetaKey  = 2
	AttrMetaSreg = 3
)

// nat 表达式
const (
	AttrNatType         = 1
	AttrNatFamily       = 2
	AttrNatSregAddrMin  = 3
	AttrNatSregAddrMax  = 4
	AttrNatSregProtoMin = 5
	AttrNatSregProtoMax = 6
)

// log 表达式
const (
	AttrLogGroup   = 1
	AttrLogSnaplen = 2
	AttrLogPrefix  = 3
	AttrLogFlags   = 4
	AttrLogLevel   = 5
)

// limit 表达式
const (
	AttrLimitRate       = 1
	AttrLimitUnit       = 2
	AttrLimitBurst      = 3
	AttrLimitBurstUnit  = 4
)

// reject 表达式
const (
	AttrRejectType     = 1
	AttrRejectIcmpCode = 2
)

// 表达式名
const (
	ExprCounter   = "counter"
	ExprImmediate = "immediate"
	ExprPayload   = "payload"
	ExprCmp       = "cmp"
	ExprMeta      = "meta"
	ExprNAT       = "nat"
	ExprLog       = "log"
	ExprAccept    = "accept"
	ExprDrop      = "drop"
	// 内核表达式名以 nft_masq 模块注册的为准，是 "masq" 不是 "masquerade"；
	// 写成 "masquerade" 内核找不到该表达式类型，返回 ENOENT（not found）。
	ExprMasq      = "masq"
	ExprReject    = "reject"
	ExprLimit     = "limit"
	ExprConntrack = "ct"
	ExprBitwise   = "bitwise"
)

// verdict
//
// 前半段复用 netfilter 的判决码（NF_DROP=0、NF_ACCEPT=1、NF_QUEUE=3），
// 后半段是 nft 自己的控制码（负值）：CONTINUE/BREAK/JUMP/GOTO/RETURN。
// 注意 NF_DROP 是 0 而不是 -1；-1 是 NFT_CONTINUE，写成 -1 会让 drop 规则变成"继续放行"。
const (
	VerdictDrop     = 0  // NF_DROP
	VerdictAccept   = 1  // NF_ACCEPT
	VerdictQueue    = 3  // NF_QUEUE
	VerdictContinue = -1 // NFT_CONTINUE
	VerdictBreak    = -2 // NFT_BREAK
	VerdictJump     = -3 // NFT_JUMP
	VerdictGoto     = -4 // NFT_GOTO
	VerdictReturn   = -5 // NFT_RETURN
)

// chain hook
const (
	HookPreRouting  = 0
	HookInput       = 1
	HookForward     = 2
	HookOutput      = 3
	HookPostRouting = 4
	HookIngress     = 5
)

// chain 类型
const (
	ChainTypeFilter = "filter"
	ChainTypeRoute  = "route"
	ChainTypeNAT    = "nat"
)

// 寄存器
const (
	RegVerdict = 0
	RegA       = 8
	RegB       = 9
	RegC       = 10
	RegD       = 11
)

// payload base
const (
	PayloadBaseLinkHeader  = 0
	PayloadBaseNetworkHeader = 1
	PayloadBaseTransportHeader = 2
)

// cmp 操作
const (
	CmpEQ = 0
	CmpNEQ = 1
	CmpLT = 2
	CmpLTE = 3
	CmpGT = 4
	CmpGTE = 5
)

// set 属性
const (
	AttrSetTable     = 1
	AttrSetName      = 2
	AttrSetFlags     = 3
	AttrSetKeyType   = 4
	AttrSetKeyLen    = 5
	AttrSetDataType  = 6
	AttrSetDataType2 = 7
	AttrSetDataLen   = 8
	AttrSetDesc      = 9
	AttrSetId        = 10
)

// 错误码
var (
	ErrExists   = errors.New("already exists")
	ErrNoEnt    = errors.New("not found")
	ErrInvalid  = errors.New("invalid argument")
	ErrNotSup   = errors.New("not supported")
)

// Conn 是一条 NETLINK_NETFILTER 套接字连接。
// 非并发安全，每个 Conn 应只在一个 goroutine 里用。
type Conn struct {
	fd  int
	seq uint32
	mu  sync.Mutex
}

// Dial 建立 nf_tables 连接。
func Dial() (*Conn, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, NETLINK_NETFILTER)
	if err != nil {
		return nil, fmt.Errorf("socket netlink: %w", err)
	}
	sa := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	if err := syscall.Bind(fd, sa); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("bind netlink: %w", err)
	}
	return &Conn{fd: fd, seq: 1000}, nil
}

// Close 关闭连接。
func (c *Conn) Close() error { return syscall.Close(c.fd) }

// Fd 返回底层文件描述符（调试用）
func (c *Conn) Fd() int { return c.fd }

// ─── 消息构造 ───

// msg 是一条待发送的 netlink 消息
type msg struct {
	msgType uint16
	flags   uint16
	body    []byte
}

// attrBuf 构造 nfnetlink 属性。
//
// 字节序规则（实测 nftdbg17 确认）：
//   - nla_len / nla_type 这两个属性头字段是**主机字节序**，不是网络字节序；
//   - nla 里承载的整数载荷（u32/u64）才是大端（网络字节序）。
//
// 早前把属性头按大端写，导致内核读到的 nla_len 荒谬（0x0a00 = 2560），
// 属性流解析失败，NEWTABLE/NEWCHAIN/NEWRULE 稳定返回 EINVAL。
type attrBuf struct {
	b []byte
}

func (a *attrBuf) begin(typ uint16) int {
	off := len(a.b)
	a.b = append(a.b, 0, 0, 0, 0)
	binary.NativeEndian.PutUint16(a.b[off+2:], typ)
	return off
}

func (a *attrBuf) end(off int) {
	// 长度字段含 4 字节头
	binary.NativeEndian.PutUint16(a.b[off:], uint16(len(a.b)-off))
	// 4 字节对齐
	for len(a.b)%4 != 0 {
		a.b = append(a.b, 0)
	}
}

func (a *attrBuf) U32(typ uint16, v uint32) {
	off := a.begin(typ)
	var x [4]byte
	binary.BigEndian.PutUint32(x[:], v)
	a.b = append(a.b, x[:]...)
	a.end(off)
}

func (a *attrBuf) U64(typ uint16, v uint64) {
	off := a.begin(typ)
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	a.b = append(a.b, x[:]...)
	a.end(off)
}

func (a *attrBuf) Str(typ uint16, s string) {
	off := a.begin(typ)
	a.b = append(a.b, []byte(s)...)
	a.b = append(a.b, 0) // 内核要求结尾 NUL，长度含 NUL
	a.end(off)
}

func (a *attrBuf) Raw(typ uint16, data []byte) {
	off := a.begin(typ)
	a.b = append(a.b, data...)
	a.end(off)
}

// Nested 嵌套属性：回调里继续 append
func (a *attrBuf) Nested(typ uint16, fn func(*attrBuf)) {
	off := a.begin(typ)
	fn(a)
	a.end(off)
}

// nfgenmsg：family(1) version(1) res_id(u16, 大端)
//
// resID 的含义随消息类型而变：
//   - BATCH_BEGIN / BATCH_END：必须是子系统号 10（NFNL_SUBSYS_NFTABLES），
//     填 0 会让内核在 nfnetlink 层直接 EOPNOTSUPP；
//   - 普通 NFT_MSG_*：填 0（gen id 由内核维护）。
//
// 实测（nftdbg12）：只有 family=0/version=0/res_id=0x000a 的 4 字节头
// 才能让 batch 被真正提交，其余组合分别得到 -95 或 -22。
func nfgen(family uint8, resID uint16) []byte {
	return []byte{family, NFNETLINK_V0, byte(resID >> 8), byte(resID)}
}

// ─── 收发 ───

// nlmsghdr 按主机字节序打包
func packHeader(length uint32, typ, flags uint16, seq uint32) []byte {
	h := make([]byte, 16)
	binary.LittleEndian.PutUint32(h[0:], length)
	binary.LittleEndian.PutUint16(h[4:], typ)
	binary.LittleEndian.PutUint16(h[6:], flags)
	binary.LittleEndian.PutUint32(h[8:], seq)
	binary.LittleEndian.PutUint32(h[12:], 0)
	return h
}

// buildBatch 组装 batch 并返回完整字节，同时返回本批占用的 seq 区间。
//
// 返回 seq 区间是必需的：内核的 ACK 可能分多个数据报回来，也可能有上一批
// 未被读走的残留 ACK 排在前面。只认本批 seq 才不会把别人的错误当自己的，
// 也不会把本批的错误漏掉（曾因此误报「建表成功但表不存在」）。
//
// 中间消息**要带** NLM_F_ACK：不带的话内核只在 BATCH_END 上回一个
// 「提交成功」，中间某条消息失败会被完全吞掉（曾因此误以为 DELTABLE 成功）。
// 实测（nftdbg17）：BEGIN/NEWTABLE/END 三条都带 ACK 时行为完全正常，
// NEWTABLE 成功回 errno=0，重复建回 -17。
func (c *Conn) buildBatch(msgs []msg) (data []byte, lo, hi uint32) {
	var out []byte

	// BATCH_BEGIN：nfgen_res_id 必须是子系统号 10，否则内核在
	// nfnetlink 层就 EOPNOTSUPP（实测 -95）。
	c.seq++
	lo = c.seq
	body := nfgen(FamilyUnspec, NFNL_SUBSYS_NFTABLES)
	out = append(out, packHeader(uint32(16+len(body)), NFNL_MSG_BATCH_BEGIN,
		syscall.NLM_F_REQUEST, c.seq)...)
	out = append(out, body...)

	// 中间消息：子系统号 10，REQUEST + ACK
	for _, m := range msgs {
		c.seq++
		out = append(out, packHeader(uint32(16+len(m.body)), m.msgType,
			m.flags|syscall.NLM_F_ACK, c.seq)...)
		out = append(out, m.body...)
	}

	// BATCH_END：同样带子系统号 10。本内核不给 BATCH_END 回 ACK，
	// 所以这里不带 NLM_F_ACK，否则会往 socket 里堆没人读的回包。
	c.seq++
	hi = c.seq
	endBody := nfgen(FamilyUnspec, NFNL_SUBSYS_NFTABLES)
	out = append(out, packHeader(uint32(16+len(endBody)), NFNL_MSG_BATCH_END,
		syscall.NLM_F_REQUEST, c.seq)...)
	out = append(out, endBody...)

	return out, lo, hi
}

// Ack 表示一条 ACK 的结果
type Ack struct {
	Seq   uint32
	Errno int32
}

// send 发送并收集属于 [lo,hi] 这批消息的 ACK
func (c *Conn) send(payload []byte, lo, hi uint32, expect int) ([]Ack, error) {
	sa := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	if err := syscall.Sendto(c.fd, payload, 0, sa); err != nil {
		return nil, fmt.Errorf("sendto: %w", err)
	}
	return c.collect(lo, hi, expect)
}

// collect 收集本批的 ACK。
//
// 三个必须点：
//  1. 不能读到第一个数据报就返回。内核会把 N 条 ACK 拆成多个数据报，
//     也可能把多条塞进一个数据报；少读一批就会把它们的错误留给下一批
//     （曾因此误判「建表成功但表不存在」）。
//  2. 必须按 seq 过滤。同一条 socket 上可能残留之前未读走的 ACK，
//     不过滤就会把旧错误当成本批错误，或把本批错误吞掉。
//  3. 结束条件用「收满 expect 条」而不是「收到 seq==hi」：本内核不给
//     BATCH_END 回 ACK，等 hi 会一直等到超时（实测每批多花 3 秒）。
//     一旦出现非零 errno 立即返回——batch 出错时内核中止后续消息，
//     剩下的 ACK 永远不会来。
func (c *Conn) collect(lo, hi uint32, expect int) ([]Ack, error) {
	tv := syscall.Timeval{Sec: 2}
	_ = syscall.SetsockoptTimeval(c.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

	var acks []Ack
	for len(acks) < expect {
		buf := make([]byte, 65536)
		n, _, err := syscall.Recvfrom(c.fd, buf, 0)
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				break
			}
			return acks, fmt.Errorf("recvfrom: %w", err)
		}
		if n <= 0 {
			break
		}
		off := 0
		for off+16 <= n {
			length := int(binary.LittleEndian.Uint32(buf[off:]))
			typ := binary.LittleEndian.Uint16(buf[off+4:])
			seq := binary.LittleEndian.Uint32(buf[off+8:])
			if length < 16 || off+length > n {
				break
			}
			if typ == syscall.NLMSG_ERROR && length >= 20 {
				code := int32(binary.LittleEndian.Uint32(buf[off+16:]))
				if seq >= lo && seq <= hi {
					acks = append(acks, Ack{Seq: seq, Errno: code})
					if code != 0 {
						return acks, nil
					}
				}
			}
			// 对齐到 4
			aligned := (length + 3) &^ 3
			off += aligned
			if aligned == 0 {
				break
			}
		}
	}
	return acks, nil
}

// wrapErrno 把内核错误码转成可读错误
func wrapErrno(code int32) error {
	switch code {
	case 0:
		return nil
	case -int32(syscall.EEXIST):
		return ErrExists
	case -int32(syscall.ENOENT):
		return ErrNoEnt
	case -int32(syscall.EINVAL):
		return ErrInvalid
	case -int32(syscall.EOPNOTSUPP):
		return ErrNotSup
	case -int32(syscall.EPERM):
		return fmt.Errorf("permission denied")
	}
	return fmt.Errorf("nft error %d", -code)
}

// runBatch 执行一批消息，任一失败即返回错误
func (c *Conn) runBatch(msgs []msg) error {
	if len(msgs) == 0 {
		return nil
	}
	data, lo, hi := c.buildBatch(msgs)
	// 只有中间消息带 NLM_F_ACK；BATCH_BEGIN 不带，BATCH_END 带了本内核也不回。
	acks, err := c.send(data, lo, hi, len(msgs))
	if err != nil {
		return err
	}
	// 按 seq 顺序取第一个非零错误：中间消息的错误更精确，
	// BATCH_END 的 ACK 只是确认提交。
	for _, a := range acks {
		if a.Errno != 0 {
			return wrapErrno(a.Errno)
		}
	}
	return nil
}

// DebugBatch 仅供调试：返回建表 batch 的原始字节
func DebugBatch(tables []Table) []byte {
	c := &Conn{fd: -1, seq: 1000}
	var msgs []msg
	for _, t := range tables {
		var a attrBuf
		a.Str(AttrTableName, t.Name)
		body := append(nfgen(t.Family, 0), a.b...)
		msgs = append(msgs, msg{
			msgType: nftType(MsgNewTable),
			flags:   syscall.NLM_F_REQUEST | syscall.NLM_F_CREATE,
			body:    body,
		})
	}
	data, _, _ := c.buildBatch(msgs)
	return data
}

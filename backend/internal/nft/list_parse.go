//go:build linux

package nft

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
)

// ─── 属性解析 ───

// Attr 是一条已解析的 netlink 属性
type Attr struct {
	Type   uint16 // 已去掉 NLA_F_NESTED / NLA_F_NET_BYTEORDER 标志位
	Raw    uint16 // 原始类型（含标志位）
	Data   []byte // 载荷（不含 4 字节头，不含对齐填充）
	Nested bool   // 是否带 NLA_F_NESTED
}

const (
	nlaTypeMask      = 0x3fff
	nlaFNested       = 0x8000
	nlaFNetByteorder = 0x4000
)

// Attrs 是一段属性流
type Attrs []Attr

// parseAttrs 解析一段属性流。
//
// 属性头 nla_len / nla_type 是「主机字节序」（与小端板子一致），
// 载荷里的整数按大端解释。
func parseAttrs(b []byte) Attrs {
	var out []Attr
	off := 0
	for off+4 <= len(b) {
		length := int(binary.NativeEndian.Uint16(b[off:]))
		rawType := binary.NativeEndian.Uint16(b[off+2:])
		if length < 4 || off+length > len(b) {
			break
		}
		out = append(out, Attr{
			Type:   rawType & nlaTypeMask,
			Raw:    rawType,
			Data:   b[off+4 : off+length],
			Nested: rawType&nlaFNested != 0,
		})
		// 4 字节对齐
		off += (length + 3) &^ 3
	}
	return Attrs(out)
}

func (as Attrs) get(typ uint16) (Attr, bool) {
	for _, a := range as {
		if a.Type == typ {
			return a, true
		}
	}
	return Attr{}, false
}

func (as Attrs) str(typ uint16) string {
	if a, ok := as.get(typ); ok {
		// 去掉结尾 NUL
		d := a.Data
		for len(d) > 0 && d[len(d)-1] == 0 {
			d = d[:len(d)-1]
		}
		return string(d)
	}
	return ""
}

func (as Attrs) u32(typ uint16) (uint32, bool) {
	if a, ok := as.get(typ); ok && len(a.Data) >= 4 {
		return binary.BigEndian.Uint32(a.Data), true
	}
	return 0, false
}

func (as Attrs) u64(typ uint16) (uint64, bool) {
	if a, ok := as.get(typ); ok && len(a.Data) >= 8 {
		return binary.BigEndian.Uint64(a.Data), true
	}
	return 0, false
}

// ─── dump 请求 ───

// dump 发送一条 DUMP 请求并返回所有 DATA 消息的（nfgenmsg 之后）载荷。
//
// 只有 GET 类消息能 dump。本内核的 dump 是多段回包（若干 DATA 后跟一个 DONE），
// 接收端必须先设 SO_RCVTIMEO，否则空结果会永久阻塞。
func (c *Conn) dump(msgType uint16, family uint8, attrs *attrBuf) ([][]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.seq++
	full := nfgen(family, 0)
	if attrs != nil {
		full = append(full, attrs.b...)
	}
	pkt := append(packHeader(uint32(16+len(full)), nftType(msgType),
		syscall.NLM_F_REQUEST|syscall.NLM_F_DUMP, c.seq), full...)

	sa := &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	if err := syscall.Sendto(c.fd, pkt, 0, sa); err != nil {
		return nil, fmt.Errorf("sendto: %w", err)
	}

	datas, errno, err := c.recvDump(c.seq)
	if err != nil {
		return nil, err
	}
	if errno != 0 {
		return nil, wrapErrno(errno)
	}
	return datas, nil
}

// recvDump 收集一个 dump 会话的全部 DATA 载荷（已跳过 nfgenmsg）。
//
// 只认本次请求 seq 的回包：同一条 socket 上可能残留前面 batch 的 ACK，
// 不过滤会把它们的 seq 误当成本次 dump 的结束信号。
func (c *Conn) recvDump(want uint32) ([][]byte, int32, error) {
	tv := syscall.Timeval{Sec: 2}
	_ = syscall.SetsockoptTimeval(c.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

	var datas [][]byte
	var errno int32
	for {
		buf := make([]byte, 65536)
		n, _, err := syscall.Recvfrom(c.fd, buf, 0)
		if err != nil {
			// 超时即认为 dump 结束
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				break
			}
			return datas, errno, fmt.Errorf("recvfrom: %w", err)
		}
		if n <= 0 {
			break
		}

		off := 0
		done := false
		for off+16 <= n {
			length := int(binary.LittleEndian.Uint32(buf[off:]))
			if length < 16 || off+length > n {
				break
			}
			typ := binary.LittleEndian.Uint16(buf[off+4:])
			seq := binary.LittleEndian.Uint32(buf[off+8:])
			payload := buf[off+16 : off+length]

			// 跳过不属于本次请求的回包
			if seq != want {
				aligned := (length + 3) &^ 3
				if aligned == 0 {
					break
				}
				off += aligned
				continue
			}

			switch typ {
			case syscall.NLMSG_DONE:
				done = true
			case syscall.NLMSG_ERROR:
				if len(payload) >= 4 {
					if code := int32(binary.LittleEndian.Uint32(payload[:4])); code != 0 {
						errno = code
					}
				}
				done = true
			case syscall.NLMSG_NOOP:
				// 忽略
			default:
				// 数据消息：前 4 字节是 nfgenmsg，跳过
				if len(payload) > 4 {
					datas = append(datas, payload[4:])
				}
			}

			aligned := (length + 3) &^ 3
			if aligned == 0 {
				break
			}
			off += aligned
		}
		if done {
			break
		}
	}
	return datas, errno, nil
}

// ─── 列举 ───

// TableInfo 是一张已存在的表
type TableInfo struct {
	Family uint8  `json:"family"`
	Name   string `json:"name"`
}

// ListTables 列出指定 family 下的所有表。
func (c *Conn) ListTables(family uint8) ([]TableInfo, error) {
	datas, err := c.dump(MsgGetTable, family, nil)
	if err != nil {
		return nil, err
	}
	var out []TableInfo
	for _, d := range datas {
		as := parseAttrs(d)
		name := as.str(AttrTableName)
		if name == "" {
			continue
		}
		out = append(out, TableInfo{Family: family, Name: name})
	}
	return out, nil
}

// ChainInfo 是一条已存在的链
type ChainInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Hook     int    `json:"hook"`
	Priority int32  `json:"priority"`
	Policy   int32  `json:"policy"`
	HasHook  bool   `json:"has_hook"`
	Packets  uint64 `json:"packets"`
	Bytes    uint64 `json:"bytes"`
}

// ListChains 列出某张表下的所有链
func (c *Conn) ListChains(t Table) ([]ChainInfo, error) {
	var a attrBuf
	a.Str(AttrChainTable, t.Name)

	datas, err := c.dump(MsgGetChain, t.Family, &a)
	if err != nil {
		return nil, err
	}
	var out []ChainInfo
	for _, d := range datas {
		as := parseAttrs(d)
		// 内核的 dump 不保证按过滤条件裁剪，这里自己按表名再筛一次，
		// 否则会把别的表的链混进来。
		if tbl := as.str(AttrChainTable); tbl != "" && tbl != t.Name {
			continue
		}
		name := as.str(AttrChainName)
		if name == "" {
			continue
		}
		ci := ChainInfo{Name: name, Type: as.str(AttrChainType)}
		if hook, ok := as.get(AttrChainHook); ok {
			inner := parseAttrs(hook.Data)
			if v, ok := inner.u32(AttrHookHooknum); ok {
				ci.Hook = int(v)
				ci.HasHook = true
			}
			if v, ok := inner.u32(AttrHookPriority); ok {
				ci.Priority = int32(v)
			}
		}
		if v, ok := as.u32(AttrChainPolicy); ok {
			ci.Policy = int32(v)
		}
		if cnt, ok := as.get(AttrChainCounter); ok {
			inner := parseAttrs(cnt.Data)
			if v, ok := inner.u64(AttrCounterBytes); ok {
				ci.Bytes = v
			}
			if v, ok := inner.u64(AttrCounterPackets); ok {
				ci.Packets = v
			}
		}
		out = append(out, ci)
	}
	return out, nil
}

// ExprInfo 是一条已解析的表达式
type ExprInfo struct {
	Name  string `json:"name"`
	Text  string `json:"text"`  // 人类可读片段，如 "tcp dport 22"
	Field string `json:"field"` // 结构化提示：dport / saddr / daddr / proto / mark / ""
	Value string `json:"value"` // 对应值
	// label 是字段本身的读法（"ip saddr"、"dport"），不带比较值；
	// 不导出，避免污染 JSON。
	label string
}

// FieldText 返回该表达式的字段读法（不含比较值）
func (e ExprInfo) FieldText() string {
	if e.label != "" {
		return e.label
	}
	return e.Text
}

func cmpOpSymbol(op uint32) string {
	switch op {
	case CmpEQ:
		return "=="
	case CmpNEQ:
		return "!="
	case CmpLT:
		return "<"
	case CmpLTE:
		return "<="
	case CmpGT:
		return ">"
	case CmpGTE:
		return ">="
	}
	return ""
}

// RuleInfo 是一条已存在的规则
type RuleInfo struct {
	Handle   uint64     `json:"handle"`
	Chain    string     `json:"chain"`
	Exprs    []ExprInfo `json:"exprs"`
	Text     string     `json:"text"` // 整条规则的读法
	Verdict  string     `json:"verdict"`
	Packets  uint64     `json:"packets"`
	Bytes    uint64     `json:"bytes"`
	HasCount bool       `json:"has_count"`
}

// ListRules 列出某张表（或某条链）下的所有规则。chain 为空表示整张表。
func (c *Conn) ListRules(t Table, chain string) ([]RuleInfo, error) {
	var a attrBuf
	a.Str(AttrRuleTable, t.Name)
	if chain != "" {
		a.Str(AttrRuleChain, chain)
	}

	datas, err := c.dump(MsgGetRule, t.Family, &a)
	if err != nil {
		return nil, err
	}
	var out []RuleInfo
	for _, d := range datas {
		as := parseAttrs(d)
		ri := RuleInfo{Chain: as.str(AttrRuleChain)}
		if v, ok := as.u64(AttrRuleHandle); ok {
			ri.Handle = v
		}
		if exprs, ok := as.get(AttrRuleExpressions); ok {
			ri.Exprs, ri.Verdict, ri.Packets, ri.Bytes, ri.HasCount = parseExprList(exprs.Data)
			ri.Text = buildRuleText(ri.Exprs, ri.Verdict)
		}
		out = append(out, ri)
	}
	return out, nil
}

// parseExprList 解析 NFTA_RULE_EXPRESSIONS 的载荷。
//
// 每个子元素是 NFTA_LIST_ELEM，内部才是 NFTA_EXPR_NAME / NFTA_EXPR_DATA。
// counter 表达式的实时值就藏在它的 data 里，这里一并取出。
func parseExprList(b []byte) (exprs []ExprInfo, verdict string, packets, bytesVal uint64, hasCount bool) {
	elems := parseAttrs(b)
	var pending []ExprInfo // meta/payload 等待 cmp 拼出完整片段
	// awaitUpper：上一条 cmp 建立了下界，同一个字段的下一条 cmp 是上界，
	// 两者合成一个区间读法。内核存的是 payload/cmp/payload/cmp 四段。
	awaitUpper := false
	// prevRaw 记住上一条 cmp 的原始字节，用来判断区间端点的大小顺序。
	var prevRaw []byte

	for _, el := range elems {
		if el.Type != AttrListElem {
			continue
		}
		inner := parseAttrs(el.Data)
		name := inner.str(AttrExprName)
		if name == "" {
			continue
		}
		var data []byte
		if d, ok := inner.get(AttrExprData); ok {
			data = d.Data
		}
		info := ExprInfo{Name: name}
		da := parseAttrs(data)

		switch name {
		case ExprCounter:
			hasCount = true
			if v, ok := da.u64(AttrCounterPackets); ok {
				packets = v
			}
			if v, ok := da.u64(AttrCounterBytes); ok {
				bytesVal = v
			}
			continue

		case ExprImmediate:
			if vd, ok := da.get(AttrImmData); ok {
				dv := parseAttrs(vd.Data)
				if verdictAttr, ok := dv.get(AttrDataVerdict); ok {
					va := parseAttrs(verdictAttr.Data)
					if code, ok := va.u32(AttrVerdictCode); ok {
						verdict = verdictName(int32(code))
					}
				}
			}
			continue

		case ExprPayload:
			base, _ := da.u32(AttrPayloadBase)
			offset, _ := da.u32(AttrPayloadOffset)
			length, _ := da.u32(AttrPayloadLen)
			info.Field, info.label = payloadField(int(base), int(offset), int(length))
			info.Text = info.label
			// 区间匹配是 payload/cmp/payload/cmp 四段：第二次读同一字段
			// 不再新起一条，而是等着和上界合并。
			if awaitUpper && len(pending) > 0 && pending[len(pending)-1].Field == info.Field {
				continue
			}
			pending = append(pending, info)
			continue

		case ExprCmp:
			op, _ := da.u32(AttrCmpOp)
			var raw []byte
			if cd, ok := da.get(AttrCmpData); ok {
				dv := parseAttrs(cd.Data)
				if v, ok := dv.get(AttrDataValue); ok {
					raw = v.Data
				}
			}
			if len(raw) == 0 {
				continue
			}
			if len(pending) == 0 {
				valStr := formatCmpValue(raw)
				pending = append(pending, ExprInfo{Name: name, Text: valStr, Value: valStr})
				prevRaw = raw
				continue
			}
			last := &pending[len(pending)-1]
			valStr := formatCmpValueFor(raw, last.Field)
			switch {
			case awaitUpper:
				// 同字段的第二次 cmp：和上一次的值构成区间。
				// 谁大谁小由原始字节比出来，不看 cmp 操作符的顺序。
				low, high := last.Value, valStr
				if compareCmpRaw(prevRaw, raw) > 0 {
					low, high = high, low
				}
				last.Value = low
				last.Text = fmt.Sprintf("%s %s-%s", last.FieldText(), low, high)
				awaitUpper = false
			case op == CmpEQ:
				last.Value = valStr
				last.Text = last.FieldText() + " " + valStr
				prevRaw = raw
			default:
				if sym := cmpOpSymbol(op); sym != "" {
					last.Value = valStr
					last.Text = last.FieldText() + " " + sym + " " + valStr
					// 下界比较（>= / >）后面通常跟同字段的上界比较
					awaitUpper = op == CmpGT || op == CmpGTE
					prevRaw = raw
				}
			}
			continue

		case ExprMeta:
			key, _ := da.u32(AttrMetaKey)
			info.Field, info.label = metaField(int(key))
			info.Text = info.label
			pending = append(pending, info)
			continue

		case ExprConntrack:
			key, _ := da.u32(AttrCtKey)
			info.Field, info.label = ctField(key)
			info.Text = info.label
			pending = append(pending, info)
			continue

		case ExprNAT:
			info.Text = "nat"
		case ExprMasq:
			info.Text = "masquerade"
		case ExprLog:
			if prefix := da.str(AttrLogPrefix); prefix != "" {
				info.Text = fmt.Sprintf("log prefix %q", prefix)
			} else {
				info.Text = "log"
			}
		case ExprReject:
			info.Text = "reject"
		case ExprLimit:
			info.Text = "limit"
		case ExprAccept:
			verdict = "accept"
			continue
		case ExprDrop:
			verdict = "drop"
			continue
		default:
			info.Text = name
		}
		pending = append(pending, info)
	}
	return pending, verdict, packets, bytesVal, hasCount
}

func buildRuleText(exprs []ExprInfo, verdict string) string {
	var parts []string
	for _, e := range exprs {
		if e.Text == "" {
			continue
		}
		parts = append(parts, e.Text)
	}
	if verdict != "" {
		parts = append(parts, verdict)
	}
	return strings.Join(parts, " ")
}

func verdictName(code int32) string {
	switch code {
	case VerdictDrop:
		return "drop"
	case VerdictAccept:
		return "accept"
	case VerdictQueue:
		return "queue"
	case VerdictContinue:
		return "continue"
	case VerdictReturn:
		return "return"
	}
	return fmt.Sprintf("verdict(%d)", code)
}

func formatCmpValue(b []byte) string {
	switch len(b) {
	case 1:
		return fmt.Sprintf("%d", b[0])
	case 2:
		return fmt.Sprintf("%d", binary.BigEndian.Uint16(b))
	case 4:
		return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
	}
	return fmt.Sprintf("%x", b)
}

// formatCmpValueFor 按字段语义格式化比较值：
// 端口要读成数字，地址要读成点分十进制，协议号顺带给出名字。
func formatCmpValueFor(b []byte, field string) string {
	switch field {
	case "sport", "dport":
		if len(b) == 2 {
			return fmt.Sprintf("%d", binary.BigEndian.Uint16(b))
		}
	case "saddr", "daddr":
		if len(b) == 4 {
			return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
		}
	case "iif", "oif":
		// ifindex 是 32 位数字，不能按 IP 读
		if len(b) == 4 {
			return fmt.Sprintf("%d", binary.BigEndian.Uint32(b))
		}
	case "iifname", "oifname":
		// 接口名是原始字节串（无结尾 NUL）
		return string(b)
	case "proto":
		if len(b) == 1 {
			if n := protoName(b[0]); n != "" {
				return n
			}
			return fmt.Sprintf("%d", b[0])
		}
	case "ctstate", "ctstatus":
		if len(b) >= 1 {
			return ctStateName(b)
		}
	}
	return formatCmpValue(b)
}

// compareCmpRaw 比较两条 cmp 的原始值，返回 -1/0/1。
// 内核 dump 出来的区间两端顺序不固定，靠数值大小判断谁是上界更可靠。
func compareCmpRaw(a, b []byte) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func protoName(p byte) string {
	switch p {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 1:
		return "icmp"
	case 58:
		return "icmpv6"
	case 132:
		return "sctp"
	}
	return ""
}

func ctStateName(b []byte) string {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	var parts []string
	for _, s := range []struct {
		bit  uint32
		name string
	}{
		{CtStateEstablished, "established"},
		{CtStateRelated, "related"},
		{CtStateNew, "new"},
		{CtStateInvalid, "invalid"},
		{CtStateUntracked, "untracked"},
	} {
		if v&s.bit != 0 {
			parts = append(parts, s.name)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ",")
}

func payloadField(base, offset, length int) (field, text string) {
	const (
		baseNet   = 1 // network header
		baseTrans = 2 // transport header
	)
	switch base {
	case baseNet:
		switch {
		case offset == 12 && length == 4:
			return "saddr", "ip saddr"
		case offset == 16 && length == 4:
			return "daddr", "ip daddr"
		case offset == 9 && length == 1:
			return "proto", "ip protocol"
		}
	case baseTrans:
		switch {
		case offset == 0 && length == 2:
			return "sport", "sport"
		case offset == 2 && length == 2:
			return "dport", "dport"
		}
	}
	return "", fmt.Sprintf("payload(base=%d,off=%d,len=%d)", base, offset, length)
}

func metaField(key int) (field, text string) {
	switch key {
	case MetaKeyL4Proto:
		return "proto", "l4proto"
	case MetaKeyMark:
		return "mark", "mark"
	case 0:
		return "length", "length"
	case MetaKeyIIF:
		return "iif", "iif"
	case MetaKeyOIF:
		return "oif", "oif"
	case MetaKeyIIFName:
		return "iifname", "iifname"
	case MetaKeyOIFName:
		return "oifname", "oifname"
	}
	return "", fmt.Sprintf("meta(key=%d)", key)
}

// ct 表达式：状态要读成 "ct state"，不是裸 "ct"
func ctField(key uint32) (field, text string) {
	switch key {
	case CtState:
		return "ctstate", "ct state"
	case CtStatus:
		return "ctstatus", "ct status"
	case CtMark:
		return "ctmark", "ct mark"
	}
	return "", "ct"
}

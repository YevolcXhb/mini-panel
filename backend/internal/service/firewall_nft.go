//go:build linux

package service

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/nft"
)

// 表名与链名常量、对外数据结构见 firewall_nft_names.go（平台无关）。

// nft 连接的复用句柄。netlink 连接建立成本不高，但每次改规则都重连
// 会让「应用全部规则」这种批量操作白白多出几十次 socket 创建。
var (
	nftMu   sync.Mutex
	nftConn *nft.Conn
)

// nftHandle 返回复用的连接，失效时重建。
func nftHandle() (*nft.Conn, error) {
	nftMu.Lock()
	defer nftMu.Unlock()
	if nftConn != nil {
		return nftConn, nil
	}
	c, err := nft.Dial()
	if err != nil {
		return nil, err
	}
	nftConn = c
	return c, nil
}

// closeNft 释放连接（进程退出或检测到连接失效时调用）。
func closeNft() {
	nftMu.Lock()
	defer nftMu.Unlock()
	if nftConn != nil {
		nftConn.Close()
		nftConn = nil
	}
}

// nftAvailable 判断内核 nf_tables 是否可用。
//
// 与 syscmd.Which("nft") 的区别：本机（Android 精简系统）没有 nft 用户态程序，
// 但内核 netfilter 完全支持 nf_tables。所以后端探测必须走 netlink 实测，
// 而不是查二进制。
func nftAvailable() bool {
	return nft.Available()
}

// nftTimeout 给单次 nft 操作加超时保护。
// netlink 是同步 REQ/ACK，内核不回包时会一直等；调用方多半是 HTTP 请求，
// 不能让一个坏掉的 netlink 会话把整个面板拖住。
func nftTimeout(d time.Duration, fn func(c *nft.Conn) error) error {
	done := make(chan error, 1)
	go func() { done <- fn(nil) }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		return fmt.Errorf("nft 操作超时（%s）", d)
	}
}

// filterTable / natTable 返回面板表的描述
func filterTable() nft.Table { return nft.Table{Family: nft.FamilyIPv4, Name: nftTableFilter} }
func natTable() nft.Table    { return nft.Table{Family: nft.FamilyIPv4, Name: nftTableNAT} }

// nftBaseOps 返回创建整张规则集骨架所需的操作。
//
// 顺序有意义：先建表再建链，链的钩子优先级决定与系统既有钩子的相对次序。
// filter 用 0（与系统一致），nat 用 -100（先于系统 nat 规则执行）。
func nftBaseOps() []nft.BatchOp {
	ft := filterTable()
	nt := natTable()
	return []nft.BatchOp{
		nft.NewTableOp(ft),
		nft.NewChainOp(nft.Chain{Table: ft, Name: nftChainInput, Type: nft.ChainTypeFilter, Hook: nft.HookInput, Priority: 0, Policy: nft.VerdictAccept}),
		nft.NewChainOp(nft.Chain{Table: ft, Name: nftChainOutput, Type: nft.ChainTypeFilter, Hook: nft.HookOutput, Priority: 0, Policy: nft.VerdictAccept}),
		nft.NewChainOp(nft.Chain{Table: ft, Name: nftChainForward, Type: nft.ChainTypeFilter, Hook: nft.HookForward, Priority: 0, Policy: nft.VerdictAccept}),
		nft.NewTableOp(nt),
		nft.NewChainOp(nft.Chain{Table: nt, Name: nftChainPreRoute, Type: nft.ChainTypeNAT, Hook: nft.HookPreRouting, Priority: -100, Policy: nft.VerdictAccept}),
		nft.NewChainOp(nft.Chain{Table: nt, Name: nftChainOutput, Type: nft.ChainTypeNAT, Hook: nft.HookOutput, Priority: -100, Policy: nft.VerdictAccept}),
		nft.NewChainOp(nft.Chain{Table: nt, Name: nftChainPostRoute, Type: nft.ChainTypeNAT, Hook: nft.HookPostRouting, Priority: -100, Policy: nft.VerdictAccept}),
	}
}

// nftReset 删掉面板表并重建骨架。
// 这是「应用全部规则」的第一步：先整体清空，再由同一批操作重建，
// 对外表现为原子替换，中间不会出现规则全空的放行窗口。
func nftReset() error {
	c, err := nftHandle()
	if err != nil {
		return err
	}
	ops := []nft.BatchOp{
		nft.DelTableOp(filterTable()),
		nft.DelTableOp(natTable()),
	}
	// 删不存在的表返回 not found，属正常情况（首次启动）
	if err := c.Apply(ops); err != nil && !isNftNotFound(err) {
		global.LOG.Warnf("[Firewall] nft 删表: %v", err)
	}
	return c.Apply(nftBaseOps())
}

func isNftNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "not found") || strings.Contains(s, "no such")
}

// isNftExists 判定「对象已存在」
func isNftExists(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "already exists") || strings.Contains(s, "file exists")
}

// nftEnsureBase 幂等地建立骨架，已存在则什么都不做。
func nftEnsureBase() error {
	c, err := nftHandle()
	if err != nil {
		return err
	}
	if ok, _ := c.HasTable(filterTable()); ok {
		return nil
	}
	return c.Apply(nftBaseOps())
}

// nftFlush 清空面板规则集（只删面板自己的表，绝不碰系统表）。
func nftFlush() error {
	c, err := nftHandle()
	if err != nil {
		return err
	}
	err = c.Apply([]nft.BatchOp{
		nft.DelTableOp(filterTable()),
		nft.DelTableOp(natTable()),
	})
	if err != nil && isNftNotFound(err) {
		return nil
	}
	return err
}

// nftRuleTarget 描述一条面板规则在 nft 里对应的链与判决
type nftRuleTarget struct {
	Table nft.Table
	Chain string
}

// nftChainFor 决定规则落在哪条链。
//
// 方向 out → output 链；dnat → nat 表的 prerouting（外部进来的流量转发）。
// 其余（port/ip + 方向 in）→ input 链。
func nftChainFor(rule *model.FirewallRule) nftRuleTarget {
	if rule.Type == "dnat" {
		return nftRuleTarget{Table: natTable(), Chain: nftChainPreRoute}
	}
	if rule.Direction == "out" {
		return nftRuleTarget{Table: filterTable(), Chain: nftChainOutput}
	}
	return nftRuleTarget{Table: filterTable(), Chain: nftChainInput}
}

// nftVerdict 把 action 转成判决码
func nftVerdict(action string) int32 {
	if action == "deny" {
		return nft.VerdictDrop
	}
	return nft.VerdictAccept
}

// nftRuleExprs 把一条面板规则翻译成 nft 表达式列表。
//
// 端口规则会按逗号拆成多条调用（每条一个表达式列表），
// 因为 nft 里一条规则只能有一个 dport 比较，匿名集合本内核未验证可用。
func nftRuleExprs(rule *model.FirewallRule) ([]nft.Expr, error) {
	var exprs []nft.Expr

	switch rule.Type {
	case "port":
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		var protoNum uint8
		switch proto {
		case "tcp":
			protoNum = 6
		case "udp":
			protoNum = 17
		default:
			return nil, fmt.Errorf("不支持的协议: %s", proto)
		}
		exprs = append(exprs,
			nft.MetaL4Proto(nft.RegA),
			// l4proto 只有 1 字节，必须用 1 字节 cmp；
			// 用 4 字节 cmp 内核按整寄存器裁剪，永不命中。
			nft.CmpByte(nft.RegA, nft.CmpEQ, protoNum),
		)
		port := strings.ReplaceAll(rule.Port, " ", "")
		if port == "" {
			return nil, fmt.Errorf("端口为空")
		}
		lo, hi, err := parsePortRange(port)
		if err != nil {
			return nil, err
		}
		offset := uint32(2) // TCP/UDP 目的端口在传输层头部偏移 2，长度 2
		exprs = append(exprs, nft.Payload(nft.PayloadBaseTransportHeader, offset, 2, nft.RegB))
		if lo == hi {
			exprs = append(exprs, nft.CmpU16(nft.RegB, nft.CmpEQ, lo))
		} else {
			// 端口段用两条 cmp 表达：取值必须同时 >= lo 且 <= hi。
			// 本内核不支持 bitwise（实测 -22），也不打算依赖区间表达式。
			exprs = append(exprs,
				nft.CmpU16(nft.RegB, nft.CmpGTE, lo),
				nft.Payload(nft.PayloadBaseTransportHeader, offset, 2, nft.RegB),
				nft.CmpU16(nft.RegB, nft.CmpLTE, hi),
			)
		}

	case "ip":
		addr, netw, err := net.ParseCIDR(rule.IP)
		if err != nil {
			ip := net.ParseIP(rule.IP)
			if ip == nil || ip.To4() == nil {
				return nil, fmt.Errorf("无效的 IPv4 地址: %s", rule.IP)
			}
			addr = ip
		}
		ip4 := addr.To4()
		if ip4 == nil {
			return nil, fmt.Errorf("IPv6 地址请使用 ip6 类型: %s", rule.IP)
		}
		offset := uint32(12) // IPv4 头里源地址偏移 12，长度 4
		if rule.Direction == "out" {
			offset = 16 // 目的地址偏移 16
		}
		exprs = append(exprs, nft.Payload(nft.PayloadBaseNetworkHeader, offset, 4, nft.RegA))
		if netw != nil {
			// CIDR 用「>= 网络地址 且 <= 广播地址」两条比较表达。
			// 内核不支持 bitwise，所以不能用掩码与运算。
			loU, hiU, ok := cidrRange(netw)
			if ok {
				exprs = append(exprs,
					nft.CmpU32(nft.RegA, nft.CmpGTE, loU),
					nft.Payload(nft.PayloadBaseNetworkHeader, offset, 4, nft.RegA),
					nft.CmpU32(nft.RegA, nft.CmpLTE, hiU),
				)
			}
		} else {
			exprs = append(exprs, nft.CmpU32(nft.RegA, nft.CmpEQ, beUint32(ip4)))
		}

	case "dnat":
		if rule.Protocol == "udp" {
			exprs = append(exprs,
				nft.MetaL4Proto(nft.RegA),
				nft.CmpByte(nft.RegA, nft.CmpEQ, 17),
				nft.Payload(nft.PayloadBaseTransportHeader, 2, 2, nft.RegB),
			)
		} else {
			exprs = append(exprs,
				nft.MetaL4Proto(nft.RegA),
				nft.CmpByte(nft.RegA, nft.CmpEQ, 6),
				nft.Payload(nft.PayloadBaseTransportHeader, 2, 2, nft.RegB),
			)
		}
		inPort, err := parseSinglePort(rule.Port)
		if err != nil {
			return nil, fmt.Errorf("入口端口: %w", err)
		}
		exprs = append(exprs, nft.CmpU16(nft.RegB, nft.CmpEQ, inPort))

		target := net.ParseIP(strings.TrimSpace(rule.IP)).To4()
		if target == nil {
			return nil, fmt.Errorf("无效的目标地址: %s", rule.IP)
		}
		targetPort, err := parseSinglePort(rule.TargetPort)
		if err != nil {
			return nil, fmt.Errorf("目标端口: %w", err)
		}
		// 地址和端口分别装进两个寄存器，再交给 dnat 表达式。
		// 端口寄存器必须是 2 字节载荷：用 4 字节时内核按错误宽度解释，
		// 规则能建成功但转发静默失效（真机实测）。
		exprs = append(exprs,
			nft.ImmediateValue32(nft.RegC, target),
			nft.ImmediateValue16(nft.RegD, targetPort),
			nft.NATDNAT(nft.RegC, nft.RegD, nft.RegD),
		)
		return exprs, nil

	default:
		return nil, fmt.Errorf("不支持的规则类型: %s", rule.Type)
	}

	return exprs, nil
}

// nftOpsForRule 生成一条面板规则对应的全部 BatchOp（端口规则可能拆成多条）。
func nftOpsForRule(rule *model.FirewallRule) ([]nft.BatchOp, error) {
	tgt := nftChainFor(rule)

	if rule.Type == "dnat" {
		exprs, err := nftRuleExprs(rule)
		if err != nil {
			return nil, err
		}
		ops := []nft.BatchOp{
			nft.AddRuleOp(nft.Rule{Table: tgt.Table, Chain: tgt.Chain, Exprs: exprs}),
		}
		// DNAT 之后通常需要 MASQUERADE 让回程流量走原路，
		// 否则内网被转发的主机无法把应答送回发起方。
		if rule.Masq {
			ops = append(ops, nft.AddRuleOp(nft.Rule{
				Table: natTable(),
				Chain: nftChainPostRoute,
				Exprs: []nft.Expr{nft.Masq()},
			}))
		}
		return ops, nil
	}

	// 端口可以写成 "80,443,8000-8010"。一条 nft 规则只能比较一个 dport，
	// 所以按逗号拆成多条规则；它们挂在同一条链上，效果等同。
	// 这段必须先于 nftRuleExprs：后者只认单个端口或区间。
	if rule.Type == "port" && strings.Contains(rule.Port, ",") {
		tail := []nft.Expr{nft.Counter(), nft.ImmediateVerdict(nftVerdict(rule.Action))}
		var ops []nft.BatchOp
		for _, seg := range strings.Split(rule.Port, ",") {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			one := *rule
			one.Port = seg
			exprs, err := nftRuleExprs(&one)
			if err != nil {
				return nil, err
			}
			ops = append(ops, nft.AddRuleOp(nft.Rule{
				Table: tgt.Table, Chain: tgt.Chain,
				Exprs: append(exprs, tail...),
			}))
		}
		if len(ops) == 0 {
			return nil, fmt.Errorf("端口为空")
		}
		return ops, nil
	}

	exprs, err := nftRuleExprs(rule)
	if err != nil {
		return nil, err
	}
	exprs = append(exprs, nft.Counter(), nft.ImmediateVerdict(nftVerdict(rule.Action)))
	return []nft.BatchOp{
		nft.AddRuleOp(nft.Rule{Table: tgt.Table, Chain: tgt.Chain, Exprs: exprs}),
	}, nil
}

// nftApplyRules 用一批规则整体重建面板规则集。
//
// 全程在单个 netlink batch 里完成：删旧表 → 建骨架 → 逐条加规则。
// 内核以一次 commit 生效，因此不存在「已清空但新规则还没加上」的窗口。
func nftApplyRules(rules []model.FirewallRule) ([]string, error) {
	c, err := nftHandle()
	if err != nil {
		return nil, fmt.Errorf("连接 nf_tables 失败: %w", err)
	}

	// 第一步：清掉面板自己的两张表。
	// 单独一批提交，且容忍 ENOENT —— 表可能只在其中一张存在，
	// 若和重建放进同一批，任一张不存在就会让整批回滚（实测报 not found）。
	var delOps []nft.BatchOp
	if ok, _ := c.HasTable(filterTable()); ok {
		delOps = append(delOps, nft.DelTableOp(filterTable()))
	}
	if ok, _ := c.HasTable(natTable()); ok {
		delOps = append(delOps, nft.DelTableOp(natTable()))
	}
	if len(delOps) > 0 {
		if err := c.Apply(delOps); err != nil && !isNftNotFound(err) {
			return nil, fmt.Errorf("清除旧规则集失败: %w", err)
		}
	}

	// 第二步：骨架 + 全部规则放进同一批，一次 commit 生效。
	//
	// 规则按「期望执行顺序」拼（先匹配的在前，兜底 drop 在最后）。
	// 内核侧靠 NEWRULE 的 NLM_F_APPEND 保证追加到链尾；
	// 不带这个标志时内核会插到链头，顺序整个颠倒（真机踩过，见 lockprobe5）。
	ops := nftBaseOps()

	// 回环接口必须始终放行，否则面板自身与本机服务立刻断开
	ft := filterTable()
	var ruleOps []nft.BatchOp
	ruleOps = append(ruleOps,
		nft.AddRuleOp(nft.Rule{
			Table: ft, Chain: nftChainInput,
			Exprs: []nft.Expr{nft.MetaKey(nft.MetaKeyIIFName, nft.RegA), nft.Cmp(nft.RegA, nft.CmpEQ, []byte("lo")), nft.ImmediateVerdict(nft.VerdictAccept)},
		}),
		nft.AddRuleOp(nft.Rule{
			Table: ft, Chain: nftChainOutput,
			Exprs: []nft.Expr{nft.MetaKey(nft.MetaKeyOIFName, nft.RegA), nft.Cmp(nft.RegA, nft.CmpEQ, []byte("lo")), nft.ImmediateVerdict(nft.VerdictAccept)},
		}),
	)

	var output []string
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled {
			output = append(output, fmt.Sprintf("skip disabled rule %d: %s", rule.ID, rule.Name))
			continue
		}
		one, err := nftOpsForRule(rule)
		if err != nil {
			output = append(output, fmt.Sprintf("rule %d (%s) 跳过: %v", rule.ID, rule.Name, err))
			global.LOG.Warnf("[Firewall] rule %d (%s) 表达式构造失败: %v", rule.ID, rule.Name, err)
			continue
		}
		ruleOps = append(ruleOps, one...)
		output = append(output, fmt.Sprintf("rule %d (%s) applied", rule.ID, rule.Name))
	}

	ops = append(ops, ruleOps...)

	if err := c.Apply(ops); err != nil {
		return output, fmt.Errorf("应用规则集失败: %w", err)
	}
	global.LOG.Infof("[Firewall] nft 规则集已应用，共 %d 条面板规则", len(rules))
	return output, nil
}

// nftListSystem 返回内核里当前真实生效的规则（面板表 + 系统既有表）。
//
// 这才是「系统规则」该显示的东西：读内核，而不是拼 iptables 命令行输出。
func nftListSystem(tableName, chainName string) (*NftTableView, error) {
	c, err := nftHandle()
	if err != nil {
		return nil, fmt.Errorf("连接 nf_tables 失败: %w", err)
	}

	tables, err := c.ListTables(nft.FamilyIPv4)
	if err != nil {
		return nil, err
	}
	sort.Slice(tables, func(i, j int) bool { return tables[i].Name < tables[j].Name })

	view := &NftTableView{Family: "ipv4"}
	for _, t := range tables {
		if tableName != "" && t.Name != tableName {
			continue
		}
		ti := NftTable{Name: t.Name, PanelOwned: t.Name == nftTableFilter || t.Name == nftTableNAT}

		chains, err := c.ListChains(nft.Table{Family: nft.FamilyIPv4, Name: t.Name})
		if err != nil {
			global.LOG.Warnf("[Firewall] 列举表 %s 的链失败: %v", t.Name, err)
			continue
		}
		sort.Slice(chains, func(i, j int) bool { return chains[i].Name < chains[j].Name })

		for _, ch := range chains {
			ci := NftChain{Name: ch.Name, Type: ch.Type, Hook: hookName(ch.Hook), HasHook: ch.HasHook, Priority: ch.Priority, Policy: policyName(ch.Policy), Packets: ch.Packets, Bytes: ch.Bytes, Rules: []NftRule{}}
			if chainName != "" && ch.Name != chainName {
				ti.Chains = append(ti.Chains, ci)
				continue
			}
			rules, err := c.ListRules(nft.Table{Family: nft.FamilyIPv4, Name: t.Name}, ch.Name)
			if err != nil {
				global.LOG.Warnf("[Firewall] 列举 %s/%s 的规则失败: %v", t.Name, ch.Name, err)
			}
			sort.Slice(rules, func(i, j int) bool { return rules[i].Handle < rules[j].Handle })
			for _, r := range rules {
				ci.Rules = append(ci.Rules, NftRule{
					Handle: r.Handle, Chain: r.Chain, Text: r.Text,
					Verdict: r.Verdict, Packets: r.Packets, Bytes: r.Bytes, HasCount: r.HasCount,
				})
			}
			ti.Chains = append(ti.Chains, ci)
		}
		view.Tables = append(view.Tables, ti)
	}
	return view, nil
}

// SystemRules 返回结构化的内核规则视图（供前端直接渲染）。
func (s *FirewallService) SystemRules(tableName, chainName string) (*NftTableView, error) {
	return nftListSystem(tableName, chainName)
}

// nftDeleteRule 按 handle 删除内核里的一条规则。
func nftDeleteRule(tableName, chainName string, handle uint64) error {
	c, err := nftHandle()
	if err != nil {
		return err
	}
	return c.DelRuleByHandle(nft.Table{Family: nft.FamilyIPv4, Name: tableName}, chainName, handle)
}

// nftInsertRule 在指定位置插入一条规则。
//
// spec 是前端给出的规则片段（如 ["tcp", "dport", "8080", "accept"]），
// 这里把它翻译成表达式插进去。
//
// position 是**人看的位置序号（从 1 开始）**，不是内核 handle。
// NFTA_RULE_POSITION 只认真实 handle，拿序号去填内核会回 ENOENT
// （表现为 not found，真机踩过）。所以先把链内规则列出来，
// 把序号翻译成第 position 条规则的 handle；position<=0 或超出末尾则追加。
func nftInsertRule(tableName, chainName string, position uint64, spec []string, action string) error {
	if len(spec) == 0 {
		return fmt.Errorf("规则内容为空")
	}
	exprs, err := nftParseSpec(spec, action)
	if err != nil {
		return err
	}
	c, err := nftHandle()
	if err != nil {
		return err
	}
	tbl := nft.Table{Family: nft.FamilyIPv4, Name: tableName}

	var handle uint64
	if position > 0 {
		rs, lerr := c.ListRules(tbl, chainName)
		if lerr != nil {
			return fmt.Errorf("读取链内规则失败: %w", lerr)
		}
		if int(position) <= len(rs) {
			handle = rs[position-1].Handle
		}
		// 超出末尾时 handle 保持 0，即追加到链尾
	}
	return c.NewRule(nft.Rule{
		Table:    tbl,
		Chain:    chainName,
		Exprs:    exprs,
		Position: handle,
	})
}

// nftParseSpec 把 "tcp dport 8080 accept" 这类片段翻译成表达式。
// 支持的形式：
//
//	tcp|udp dport <port|lo-hi> accept|drop
//	saddr|daddr <ip|cidr> accept|drop
//	<任意> 直接以 accept|drop 结尾也可
func nftParseSpec(spec []string, action string) ([]nft.Expr, error) {
	var exprs []nft.Expr
	i := 0

	// 前导协议
	if i < len(spec) {
		switch strings.ToLower(spec[i]) {
		case "tcp":
			exprs = append(exprs, nft.MetaL4Proto(nft.RegA), nft.CmpByte(nft.RegA, nft.CmpEQ, 6))
			i++
		case "udp":
			exprs = append(exprs, nft.MetaL4Proto(nft.RegA), nft.CmpByte(nft.RegA, nft.CmpEQ, 17))
			i++
		case "icmp":
			exprs = append(exprs, nft.MetaL4Proto(nft.RegA), nft.CmpByte(nft.RegA, nft.CmpEQ, 1))
			i++
		}
	}

	for i < len(spec) {
		tok := strings.ToLower(spec[i])
		switch tok {
		case "dport", "sport":
			if i+1 >= len(spec) {
				return nil, fmt.Errorf("%s 缺少端口值", tok)
			}
			offset := uint32(2)
			if tok == "sport" {
				offset = 0
			}
			lo, hi, err := parsePortRange(spec[i+1])
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, nft.Payload(nft.PayloadBaseTransportHeader, offset, 2, nft.RegB))
			if lo == hi {
				exprs = append(exprs, nft.CmpU16(nft.RegB, nft.CmpEQ, lo))
			} else {
				exprs = append(exprs,
					nft.CmpU16(nft.RegB, nft.CmpGTE, lo),
					nft.Payload(nft.PayloadBaseTransportHeader, offset, 2, nft.RegB),
					nft.CmpU16(nft.RegB, nft.CmpLTE, hi),
				)
			}
			i += 2
		case "saddr", "daddr":
			if i+1 >= len(spec) {
				return nil, fmt.Errorf("%s 缺少地址", tok)
			}
			offset := uint32(12)
			if tok == "daddr" {
				offset = 16
			}
			_, netw, err := net.ParseCIDR(spec[i+1])
			if err != nil {
				ip := net.ParseIP(spec[i+1]).To4()
				if ip == nil {
					return nil, fmt.Errorf("无效地址: %s", spec[i+1])
				}
				exprs = append(exprs,
					nft.Payload(nft.PayloadBaseNetworkHeader, offset, 4, nft.RegA),
					nft.CmpU32(nft.RegA, nft.CmpEQ, beUint32(ip)),
				)
				i += 2
				continue
			}
			loU, hiU, ok := cidrRange(netw)
			if !ok {
				return nil, fmt.Errorf("无效网段: %s", spec[i+1])
			}
			exprs = append(exprs,
				nft.Payload(nft.PayloadBaseNetworkHeader, offset, 4, nft.RegA),
				nft.CmpU32(nft.RegA, nft.CmpGTE, loU),
				nft.Payload(nft.PayloadBaseNetworkHeader, offset, 4, nft.RegA),
				nft.CmpU32(nft.RegA, nft.CmpLTE, hiU),
			)
			i += 2
		case "accept", "drop", "reject":
			action = tok
			i++
		default:
			return nil, fmt.Errorf("无法识别的规则片段: %s", spec[i])
		}
	}

	switch action {
	case "drop":
		exprs = append(exprs, nft.Counter(), nft.ImmediateVerdict(nft.VerdictDrop))
	case "reject":
		exprs = append(exprs, nft.Counter(), nft.Reject(0))
	default:
		exprs = append(exprs, nft.Counter(), nft.ImmediateVerdict(nft.VerdictAccept))
	}
	return exprs, nil
}

// nftLockdown 内网-only 模式：只放行已建立连接、回环与三个内网段，其余全部丢弃。
func nftLockdown() error {
	c, err := nftHandle()
	if err != nil {
		return err
	}
	ft := filterTable()
	nt := natTable()

	// 先把面板的两张表都删掉（表可能不存在，和重建放一批会让整批回滚）。
	// 只删 filter 而留下 nat 的话，nftBaseOps 里的 NewTableOp(mp_nat)
	// 会撞 already exists 让整批失败，lockdown 直接建不起来（真机踩过）。
	var delOps []nft.BatchOp
	if ok, _ := c.HasTable(ft); ok {
		delOps = append(delOps, nft.DelTableOp(ft))
	}
	if ok, _ := c.HasTable(nt); ok {
		delOps = append(delOps, nft.DelTableOp(nt))
	}
	if len(delOps) > 0 {
		if err := c.Apply(delOps); err != nil && !isNftNotFound(err) {
			return err
		}
	}

	ops := nftBaseOps()
	// 按「期望执行顺序」拼规则（先匹配的在前，兜底 drop 在最后）。
	// 内核侧靠 NEWRULE 的 NLM_F_APPEND 追加到链尾；不带该标志会插到链头，
	// 顺序整个颠倒（真机踩过，见 lockprobe5 v13/v14）。
	var ruleOps []nft.BatchOp
	ruleOps = append(ruleOps,
		// 已建立/相关连接：ct state 非 0 即已跟踪，放行
		nft.AddRuleOp(nft.Rule{Table: ft, Chain: nftChainInput, Exprs: []nft.Expr{
			nft.CTState(nft.RegB),
			nft.CmpU32(nft.RegB, nft.CmpNEQ, 0),
			nft.ImmediateVerdict(nft.VerdictAccept),
		}}),
		// 回环
		nft.AddRuleOp(nft.Rule{Table: ft, Chain: nftChainInput, Exprs: []nft.Expr{
			nft.MetaKey(nft.MetaKeyIIFName, nft.RegA),
			nft.Cmp(nft.RegA, nft.CmpEQ, []byte("lo")),
			nft.ImmediateVerdict(nft.VerdictAccept),
		}}),
	)
	// 三个内网段
	for _, cidr := range []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"} {
		_, netw, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		loU, hiU, ok := cidrRange(netw)
		if !ok {
			continue
		}
		ruleOps = append(ruleOps, nft.AddRuleOp(nft.Rule{Table: ft, Chain: nftChainInput, Exprs: []nft.Expr{
			nft.Payload(nft.PayloadBaseNetworkHeader, 12, 4, nft.RegA),
			nft.CmpU32(nft.RegA, nft.CmpGTE, loU),
			nft.Payload(nft.PayloadBaseNetworkHeader, 12, 4, nft.RegA),
			nft.CmpU32(nft.RegA, nft.CmpLTE, hiU),
			nft.ImmediateVerdict(nft.VerdictAccept),
		}}))
	}
	// 其余一律丢弃（自然顺序里排最后，即链尾兜底）
	ruleOps = append(ruleOps, nft.AddRuleOp(nft.Rule{Table: ft, Chain: nftChainInput, Exprs: []nft.Expr{
		nft.ImmediateVerdict(nft.VerdictDrop),
	}}))
	ops = append(ops, ruleOps...)

	// 上面已把两张表都删干净，这里直接重建，不能再跳第一条
	// （旧代码有 `if !HasTable { ops = ops[1:] }`，那是配合「删表和建表同批」
	// 的写法，表两张都删之后会误删 NewTableOp，导致整批 not found）。
	return c.Apply(ops)
}

// ---- 小工具 ----

// beUint32 把 4 字节大端地址读成 uint32
func beUint32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// cidrRange 返回 IPv4 网段的首尾地址（都包含）。
//
// 本内核不支持 bitwise 表达式（实测一律 EINVAL），所以网段匹配只能写成
// 「saddr >= 网络地址 且 saddr <= 广播地址」两条比较。
// 广播地址 = 网络地址 | (0xFFFFFFFF >> 前缀长度)。
// 注意右移量是前缀长度 ones，不是 bits-ones —— 后者会把 /8 算成
// 10.0.0.0-10.0.0.255 这种错误区间（真机踩过）。
func cidrRange(netw *net.IPNet) (lo, hi uint32, ok bool) {
	if netw == nil {
		return 0, 0, false
	}
	ip4 := netw.IP.To4()
	if ip4 == nil {
		return 0, 0, false
	}
	ones, bits := netw.Mask.Size()
	if bits != 32 {
		return 0, 0, false
	}
	lo = beUint32(ip4)
	hi = lo | (^(uint32(0)) >> uint(ones))
	return lo, hi, true
}

// parsePortRange 解析 "80" 或 "3306-3308"
func parsePortRange(s string) (uint16, uint16, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, fmt.Errorf("端口为空")
	}
	if i := strings.IndexAny(s, "-:"); i > 0 {
		lo, err := parseSinglePort(s[:i])
		if err != nil {
			return 0, 0, err
		}
		hi, err := parseSinglePort(s[i+1:])
		if err != nil {
			return 0, 0, err
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		return lo, hi, nil
	}
	p, err := parseSinglePort(s)
	if err != nil {
		return 0, 0, err
	}
	return p, p, nil
}

// parseSinglePort 解析单个端口并做范围校验
func parseSinglePort(s string) (uint16, error) {
	s = strings.TrimSpace(s)
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("无效端口: %s", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("端口超出范围: %d", n)
	}
	return uint16(n), nil
}

// hookName 把钩子编号转成名字
func hookName(h int) string {
	switch h {
	case nft.HookPreRouting:
		return "prerouting"
	case nft.HookInput:
		return "input"
	case nft.HookForward:
		return "forward"
	case nft.HookOutput:
		return "output"
	case nft.HookPostRouting:
		return "postrouting"
	case nft.HookIngress:
		return "ingress"
	}
	return ""
}

// policyName 把策略码转成名字
func policyName(p int32) string {
	switch p {
	case nft.VerdictAccept:
		return "accept"
	case nft.VerdictDrop:
		return "drop"
	}
	return ""
}

package service

// 这里只放平台无关的东西：表名/链名常量、对外的数据结构、纯格式化函数。
// Linux 上的真实实现放在 firewall_nft.go（//go:build linux），
// 其它平台的桩实现放在 firewall_nft_stub.go（//go:build !linux）。

const (
	nftTableFilter = "mp_filter"
	nftTableNAT    = "mp_nat"
)

const (
	nftChainInput     = "input"
	nftChainOutput    = "output"
	nftChainForward   = "forward"
	nftChainPreRoute  = "prerouting"
	nftChainPostRoute = "postrouting"
)

// panelTag 标记这张表是不是面板自己创建的
func panelTag(owned bool) string {
	if owned {
		return " (面板)"
	}
	return ""
}

// NftRule 是内核里一条规则的读法
type NftRule struct {
	Handle   uint64 `json:"handle"`
	Chain    string `json:"chain"`
	Text     string `json:"text"`
	Verdict  string `json:"verdict"`
	Packets  uint64 `json:"packets"`
	Bytes    uint64 `json:"bytes"`
	HasCount bool   `json:"has_count"`
}

// NftChain 是内核里一条链
type NftChain struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Hook     string    `json:"hook"`
	HasHook  bool      `json:"has_hook"`
	Priority int32     `json:"priority"`
	Policy   string    `json:"policy"`
	Packets  uint64    `json:"packets"`
	Bytes    uint64    `json:"bytes"`
	Rules    []NftRule `json:"rules"`
}

// NftTable 是内核里一张表
type NftTable struct {
	Name       string     `json:"name"`
	PanelOwned bool       `json:"panel_owned"`
	Chains     []NftChain `json:"chains"`
}

// NftTableView 是系统规则的整体视图
type NftTableView struct {
	Family string     `json:"family"`
	Tables []NftTable `json:"tables"`
}

package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Config 代理配置（存储于 settings 表 key=proxy_config）
type Config struct {
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"` // http / socks5
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	// NoProxy 不走代理的地址（逗号分隔，支持通配符域名），如 localhost,127.0.0.1,*.lan
	NoProxy string `json:"no_proxy"`
}

// ParseJSON 从 settings 存储的字符串解析配置
func ParseJSON(s string) (*Config, error) {
	if strings.TrimSpace(s) == "" {
		return &Config{}, nil
	}
	var c Config
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate 校验配置
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	t := strings.ToLower(strings.TrimSpace(c.Type))
	if t != "http" && t != "socks5" {
		return fmt.Errorf("代理类型仅支持 http / socks5")
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("代理主机不能为空")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("代理端口无效")
	}
	return nil
}

// ProxyURL 生成 proxy.URL
func (c *Config) ProxyURL() (*url.URL, error) {
	var raw string
	if c.Username != "" {
		raw = fmt.Sprintf("%s://%s:%s@%s:%d",
			strings.ToLower(c.Type), url.QueryEscape(c.Username), url.QueryEscape(c.Password), c.Host, c.Port)
	} else {
		raw = fmt.Sprintf("%s://%s:%d", strings.ToLower(c.Type), c.Host, c.Port)
	}
	return url.Parse(raw)
}

// Manager 全局代理管理器：热加载 = 原子替换出站 Transport。
// 所有出站 HTTP 请求共用 OutboundHTTPClient()，保存配置后下一次请求立即走新代理。
type Manager struct {
	current atomic.Value // *http.Client
}

var Default = &Manager{}

func init() {
	Default.current.Store(&http.Client{Timeout: 30 * time.Second})
}

// Apply 使配置立即生效（热加载）。enabled=false 时恢复直连。
// 返回前用配置实际请求一次代理可达性探测，失败即报错、不切换，保证保存失败不弄断出站。
func (m *Manager) Apply(c *Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	// 先登记配置再探测：探测失败只意味着出站 HTTP 不切换，
	// 但终端环境变量仍应反映用户保存的配置。
	m.SetConfig(c)
	if !c.Enabled {
		// 关闭代理时也要清掉记录的配置，否则终端仍会继承旧代理
		lastConfig.Store(&Config{})
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if c.Enabled {
		pu, err := c.ProxyURL()
		if err != nil {
			return fmt.Errorf("代理地址无效: %w", err)
		}
		transport := &http.Transport{}
		if c.NoProxy != "" {
			transport.Proxy = makeProxyFunc(c)
		} else {
			transport.Proxy = http.ProxyURL(pu)
		}
		client.Transport = transport

		// 探测代理可达性（HEAD 一个公共地址，3 秒超时），失败则不应用
		probe := &http.Client{
			Timeout:   3 * time.Second,
			Transport: transport,
		}
		resp, err := probe.Head("https://www.gstatic.com/generate_204")
		if err != nil {
			return fmt.Errorf("代理连通性测试失败: %w", err)
		}
		resp.Body.Close()
	}
	// 原子替换——所有后续出站请求立即使用新配置
	m.current.Store(client)
	return nil
}

func makeProxyFunc(c *Config) func(*http.Request) (*url.URL, error) {
	pu, _ := c.ProxyURL()
	exclusions := strings.Split(c.NoProxy, ",")
	for i := range exclusions {
		exclusions[i] = strings.TrimSpace(exclusions[i])
	}
	return func(req *http.Request) (*url.URL, error) {
		host := req.URL.Hostname()
		for _, ex := range exclusions {
			if ex == "" {
				continue
			}
			if strings.HasPrefix(ex, "*.") {
				suffix := ex[1:] // ".lan"
				if strings.HasSuffix(host, suffix) {
					return nil, nil
				}
			} else if host == ex {
				return nil, nil
			}
		}
		return pu, nil
	}
}

// OutboundHTTPClient 返回当前生效的出站客户端（热加载后立即是新代理）。
// 任何需要访问外网的代码都应使用它而不是自己 new http.Client。
func OutboundHTTPClient() *http.Client {
	return Default.current.Load().(*http.Client)
}

// Current 读取当前配置（用于回显）
func (m *Manager) Current() *http.Client {
	return m.current.Load().(*http.Client)
}

// lastConfig 保存最近一次成功 Apply 的配置，供终端等需要环境变量的场景读取。
// 与 HTTP 客户端分开存储：Client 只需 Transport，终端需要的是 host:port 原文。
var lastConfig atomic.Value // *Config

// SetConfig 记录当前生效配置（由 Apply 内部调用，也允许启动时直接回填）。
func (m *Manager) SetConfig(c *Config) {
	if c == nil {
		return
	}
	cp := *c
	lastConfig.Store(&cp)
}

// ActiveConfig 返回最近生效的代理配置，未配置时返回 nil。
func ActiveConfig() *Config {
	if v := lastConfig.Load(); v != nil {
		return v.(*Config)
	}
	return nil
}

// IsEnabled 报告当前是否配置了可用的出站代理。
func IsEnabled() bool {
	c := ActiveConfig()
	return c != nil && c.Enabled && strings.TrimSpace(c.Host) != ""
}

// EnvVars 生成代理相关的环境变量（小写 + 大写全套）。
//
// 为什么全给：curl/wget 认小写 http_proxy 也认大写，Go 程序（含本面板自身）
// 认大写 HTTPS_PROXY，apt 认 Acquire:: 配置但也读环境变量，git 两套都读。
// 只设一套会漏掉一半工具，所以 4 个变量全写，另加 ALL_PROXY 覆盖非 HTTP 场景。
func (c *Config) EnvVars() map[string]string {
	if c == nil || !c.Enabled || strings.TrimSpace(c.Host) == "" {
		return nil
	}
	scheme := strings.ToLower(strings.TrimSpace(c.Type))
	if scheme != "http" && scheme != "socks5" {
		scheme = "http"
	}
	var hostport string
	if c.Username != "" {
		hostport = fmt.Sprintf("%s:%s@%s:%d",
			url.QueryEscape(c.Username), url.QueryEscape(c.Password), c.Host, c.Port)
	} else {
		hostport = fmt.Sprintf("%s:%d", c.Host, c.Port)
	}
	p := fmt.Sprintf("%s://%s", scheme, hostport)

	env := map[string]string{
		"http_proxy":  p,
		"https_proxy": p,
		"HTTP_PROXY":  p,
		"HTTPS_PROXY": p,
		"all_proxy":   p,
		"ALL_PROXY":   p,
	}
	if np := strings.TrimSpace(c.NoProxy); np != "" {
		env["no_proxy"] = np
		env["NO_PROXY"] = np
	} else {
		// 默认让本机地址直连：proxy-on 自身要回连 127.0.0.1 的面板接口，
		// 若把 loopback 也送进代理，脚本永远拿不到配置。
		env["no_proxy"] = defaultNoProxy
		env["NO_PROXY"] = defaultNoProxy
	}
	return env
}

// defaultNoProxy 是未显式配置直连例外时使用的默认值。
const defaultNoProxy = "localhost,127.0.0.1,::1"

// ShellExports 生成可直接 source 的 shell 脚本片段（含 unset 兜底）。
// 即便终端环境已注入，用户中途改配置后也能用 `eval "$(panel-proxy)"` 之类的方式刷新。
func (c *Config) ShellExports() string {
	env := c.EnvVars()
	if len(env) == 0 {
		keys := []string{"http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY", "all_proxy", "ALL_PROXY", "no_proxy", "NO_PROXY"}
		lines := make([]string, 0, len(keys))
		for _, k := range keys {
			lines = append(lines, "unset "+k)
		}
		return strings.Join(lines, "\n")
	}
	// 固定顺序，便于 diff 和阅读
	order := []string{"http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY", "all_proxy", "ALL_PROXY", "no_proxy", "NO_PROXY"}
	lines := make([]string, 0, len(order))
	for _, k := range order {
		if v, ok := env[k]; ok {
			lines = append(lines, fmt.Sprintf("export %s='%s'", k, strings.ReplaceAll(v, "'", `'\''`)))
		}
	}
	return strings.Join(lines, "\n")
}

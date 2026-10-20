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

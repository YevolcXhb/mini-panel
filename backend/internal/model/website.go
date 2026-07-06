package model

type Website struct {
	BaseModel
	Name        string `json:"name" gorm:"not null"`
	Domain      string `json:"domain" gorm:"not null;index"`
	Port        int    `json:"port" gorm:"default:80"`
	Root        string `json:"root"`
	Type        string `json:"type" gorm:"default:static"` // static / proxy / php
	ProxyTarget string `json:"proxy_target"`               // e.g. http://localhost:8080
	ProxyWS     bool   `json:"proxy_ws"`                   // WebSocket 支持
	PhpVersion  string `json:"php_version"`                 // PHP 版本，如 8.1
	SSL         bool   `json:"ssl" gorm:"default:false"`
	SSLCert     string `json:"ssl_cert"`                      // 证书文件路径（兼容旧版）
	SSLKey      string `json:"ssl_key"`                       // 私钥文件路径（兼容旧版）
	SSLCertPEM  string `json:"ssl_cert_pem" gorm:"type:text"` // 证书 PEM 内容
	SSLKeyPEM   string `json:"ssl_key_pem" gorm:"type:text"`  // 私钥 PEM 内容
	IndexPage   string `json:"index_page" gorm:"default:index.html index.htm index.php"`
	// 301/302 重定向 (JSON数组: [{"from":"example.com","to":"www.example.com","code":301}])
	Redirects string `json:"redirects" gorm:"type:text"`
	// 目录密码保护
	AuthEnabled  bool   `json:"auth_enabled" gorm:"default:false"`
	AuthUser     string `json:"auth_user"`     // 用户名
	AuthPassword string `json:"auth_password"` // 密码（明文，生成 htpasswd 后写入）
	Enabled      bool   `json:"enabled" gorm:"default:true"`
	Managed      bool   `json:"managed" gorm:"default:true"`
	ConfigFile   string `json:"config_file"`
	Remark       string `json:"remark"`
	// 自定义错误页面
	ErrorPage404 string `json:"error_page_404" gorm:"type:text"`
	ErrorPage502 string `json:"error_page_502" gorm:"type:text"`
	ErrorPage503 string `json:"error_page_503" gorm:"type:text"`
	// 频率限制
	RateLimitEnabled bool   `json:"rate_limit_enabled" gorm:"default:false"`
	RateLimitRate    string `json:"rate_limit_rate"`    // "10r/s"
	RateLimitBurst   int    `json:"rate_limit_burst" gorm:"default:10"`
	// 防盗链
	HotlinkProtection bool   `json:"hotlink_protection" gorm:"default:false"`
	HotlinkDomains    string `json:"hotlink_domains"`                                 // 允许的域名，逗号分隔
	HotlinkExts       string `json:"hotlink_exts" gorm:"default:jpg,jpeg,png,gif,svg,webp,js,css,woff,woff2"`
	// IP 黑白名单
	IPFilterEnabled bool   `json:"ip_filter_enabled" gorm:"default:false"`
	IPFilterMode    string `json:"ip_filter_mode"`     // "blacklist" / "whitelist"
	IPFilterList    string `json:"ip_filter_list" gorm:"type:text"` // 换行分隔的 IP/CIDR
}

// RedirectRule 重定向规则
type RedirectRule struct {
	From string `json:"from"` // 源域名/路径
	To   string `json:"to"`   // 目标 URL
	Code int    `json:"code"` // 301 或 302
}

type NginxStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version"`
	ConfigDir string `json:"config_dir"`
	Pid       int    `json:"pid"`
}

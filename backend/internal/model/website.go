package model

type Website struct {
	BaseModel
	Name        string `json:"name" gorm:"not null"`
	Domain      string `json:"domain" gorm:"not null;index"`
	Port        int    `json:"port" gorm:"default:80"`
	Root        string `json:"root"`
	Type        string `json:"type" gorm:"default:static"` // static / proxy
	ProxyTarget string `json:"proxy_target"`               // e.g. http://localhost:8080
	SSL         bool   `json:"ssl" gorm:"default:false"`
	SSLCert     string `json:"ssl_cert"`
	SSLKey      string `json:"ssl_key"`
	Enabled     bool   `json:"enabled" gorm:"default:true"`
	Managed     bool   `json:"managed" gorm:"default:true"` // true: 面板管理, false: 外部创建只读
	ConfigFile  string `json:"config_file"`                 // 配置文件路径
	Remark      string `json:"remark"`
}

type NginxStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version"`
	ConfigDir string `json:"config_dir"`
	Pid       int    `json:"pid"`
}

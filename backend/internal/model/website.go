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
	Remark      string `json:"remark"`
}

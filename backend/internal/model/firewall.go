package model

import (
	"time"

	"gorm.io/gorm"
)

type FirewallRule struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	Name      string         `json:"name" gorm:"not null"`
	Type      string         `json:"type" gorm:"not null"`   // port, ip, dnat
	Action    string         `json:"action" gorm:"not null"` // allow, deny
	Protocol  string         `json:"protocol"`               // tcp, udp, all
	Port      string         `json:"port"`                   // 80, 443, 3306-3308
	IP        string         `json:"ip"`
	Direction string         `json:"direction" gorm:"default:in"` // in, out
	// DNAT 端口转发专用字段
	TargetPort string `json:"target_port"`                     // 目标端口
	Chain      string `json:"chain" gorm:"default:PREROUTING"` // PREROUTING / oem_nat_pre
	Masq       bool   `json:"masq" gorm:"default:true"`        // 自动添加回程 MASQUERADE
	Enabled    bool   `json:"enabled" gorm:"default:true"`
	Note       string `json:"note"`
}

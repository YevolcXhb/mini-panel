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
	Type      string         `json:"type" gorm:"not null"`      // port, ip
	Action    string         `json:"action" gorm:"not null"`    // allow, deny
	Protocol  string         `json:"protocol"`                  // tcp, udp, all
	Port      string         `json:"port"`                      // 80, 443, 3306-3308
	IP        string         `json:"ip"`
	Direction string         `json:"direction" gorm:"default:in"` // in, out
	Enabled   bool           `json:"enabled" gorm:"default:true"`
	Note      string         `json:"note"`
}

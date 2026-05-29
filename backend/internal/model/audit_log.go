package model

import (
	"gorm.io/gorm"
)

type AuditLog struct {
	gorm.Model
	Username  string `gorm:"index;not null" json:"username"`
	Action    string `gorm:"index;not null" json:"action"`
	Resource  string `json:"resource"`
	Detail    string `json:"detail"`
	IP        string `json:"ip"`
	Success   bool   `json:"success"`
}

package model

import "gorm.io/gorm"

type DatabaseInstance struct {
	gorm.Model
	Name     string `json:"name" gorm:"uniqueIndex;not null"`
	Type     string `json:"type" gorm:"not null"` // mysql, postgresql, redis, mongodb
	Host     string `json:"host" gorm:"not null"`
	Port     int    `json:"port" gorm:"not null"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSL      bool   `json:"ssl"`
	Enabled  bool   `json:"enabled" gorm:"default:true"`
	Note     string `json:"note"`
}

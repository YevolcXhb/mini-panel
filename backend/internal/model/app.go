package model

import (
	"gorm.io/gorm"
)

type App struct {
	gorm.Model
	Name        string `gorm:"not null" json:"name"`
	Image       string `gorm:"not null" json:"image"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Version     string `json:"version"`
	Icon        string `json:"icon"`
	EnvVars     string `json:"env_vars"`
	Volumes     string `json:"volumes"`
	Command     string `json:"command"`
}

type AppInstall struct {
	gorm.Model
	AppID     uint   `json:"app_id"`
	Name      string `gorm:"uniqueIndex;not null" json:"name"`
	Status    string `gorm:"default:installing" json:"status"`
	Image     string `json:"image"`
	Container string `json:"container"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
}

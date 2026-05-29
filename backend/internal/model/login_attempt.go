package model

import (
	"time"

	"gorm.io/gorm"
)

type LoginAttempt struct {
	gorm.Model
	Username  string    `gorm:"index;not null" json:"username"`
	IP        string    `gorm:"index;not null" json:"ip"`
	Success   bool      `json:"success"`
	LockedUntil *time.Time `json:"locked_until,omitempty"`
}

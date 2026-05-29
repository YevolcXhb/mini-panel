package model

import "time"

type LoginAttempt struct {
	BaseModel
	Username    string     `gorm:"index;not null" json:"username"`
	IP          string     `gorm:"index;not null" json:"ip"`
	Success     bool       `json:"success"`
	LockedUntil *time.Time `json:"locked_until,omitempty"`
}

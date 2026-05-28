package model

import (
	"gorm.io/gorm"
)

type Setting struct {
	gorm.Model
	Key   string `gorm:"uniqueIndex;not null" json:"key"`
	Value string `json:"value"`
}

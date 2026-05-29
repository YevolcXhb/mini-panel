package model

type Setting struct {
	BaseModel
	Key   string `gorm:"uniqueIndex;not null" json:"key"`
	Value string `json:"value"`
}

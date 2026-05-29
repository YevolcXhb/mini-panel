package model

type Cronjob struct {
	BaseModel
	Name     string `gorm:"not null" json:"name"`
	Spec     string `gorm:"not null" json:"spec"`
	Command  string `gorm:"not null" json:"command"`
	Script   string `json:"script"`
	Status   string `gorm:"default:enabled" json:"status"`
	EntryID  int    `json:"entry_id"`
	LastRun  int64  `json:"last_run"`
	LastLog  string `json:"last_log"`
}

package model

type DatabaseInstance struct {
	BaseModel
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

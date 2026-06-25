package model

type App struct {
	BaseModel
	Key         string `gorm:"uniqueIndex;not null" json:"key"`
	Name        string `gorm:"not null" json:"name"`
	Description string `json:"description"`
	ShortDesc   string `json:"short_desc"`
	Icon        string `json:"icon"`
	Category    string `gorm:"index" json:"category"`
	Type        string `gorm:"default:'container'" json:"type"`
	Status      string `gorm:"default:'active'" json:"status"`
	Website     string `json:"website"`
	Github      string `json:"github"`
	Document    string `json:"document"`
	Resource    string `gorm:"default:'remote'" json:"resource"`
	SourceID    uint   `json:"source_id"`
}

type AppDetail struct {
	BaseModel
	AppID       uint   `gorm:"index;not null" json:"app_id"`
	Version     string `gorm:"not null" json:"version"`
	Image       string `gorm:"not null" json:"image"`
	DownloadURL string `json:"download_url"`
	EnvVars     string `json:"env_vars"`
	Volumes     string `json:"volumes"`
	Command     string `json:"command"`
	Params      string `json:"params"`
	Status      string `gorm:"default:'active'" json:"status"`
}

type AppInstall struct {
	BaseModel
	AppID       uint   `gorm:"index;not null" json:"app_id"`
	AppDetailID uint   `gorm:"index;not null" json:"app_detail_id"`
	Name        string `gorm:"uniqueIndex;not null" json:"name"`
	Status      string `gorm:"default:'installing'" json:"status"`
	Progress    int    `gorm:"default:0" json:"progress"`
	Image       string `json:"image"`
	Version     string `json:"version"`
	Container   string `json:"container"`
	Port        int    `json:"port"`
	Path        string `json:"path"`
	Env         string `json:"env"`
	Message     string `json:"message"`
}

type AppSource struct {
	BaseModel
	Name    string `gorm:"not null" json:"name"`
	URL     string `gorm:"not null" json:"url"`
	Enabled bool   `gorm:"default:true" json:"enabled"`
}

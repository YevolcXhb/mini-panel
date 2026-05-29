package dto

type AppInstallRequest struct {
	AppID       uint              `json:"app_id" binding:"required"`
	AppDetailID uint              `json:"app_detail_id"`
	Name        string            `json:"name" binding:"required"`
	Port        int               `json:"port"`
	Env         map[string]string `json:"env"`
	Volumes     map[string]string `json:"volumes"`
}

type AppInstallResponse struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Port   int    `json:"port"`
}

type RemoteApp struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ShortDesc   string          `json:"short_desc"`
	Icon        string          `json:"icon"`
	Category    string          `json:"category"`
	Type        string          `json:"type"`
	Website     string          `json:"website"`
	Github      string          `json:"github"`
	Document    string          `json:"document"`
	Versions    []RemoteVersion `json:"versions"`
}

type RemoteVersion struct {
	Version string `json:"version"`
	Image   string `json:"image"`
	EnvVars string `json:"env_vars"`
	Volumes string `json:"volumes"`
	Command string `json:"command"`
	Params  string `json:"params"`
}

type AppSyncRequest struct {
	SourceID uint `json:"source_id" binding:"required"`
}

type AppSourceRequest struct {
	Name string `json:"name" binding:"required"`
	URL  string `json:"url" binding:"required"`
}

package dto

type AppInstallRequest struct {
	AppID uint   `json:"app_id" binding:"required"`
	Name  string `json:"name" binding:"required"`
	Port  int    `json:"port"`
}

type AppInstallResponse struct {
	ID     uint   `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Port   int    `json:"port"`
}

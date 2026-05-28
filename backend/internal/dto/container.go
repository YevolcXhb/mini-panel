package dto

type ContainerCreateRequest struct {
	Name    string   `json:"name" binding:"required"`
	Image   string   `json:"image" binding:"required"`
	Command string   `json:"command"`
	Env     []string `json:"env"`
	Volumes []string `json:"volumes"`
	Detach  bool     `json:"detach"`
}

type ContainerListResponse struct {
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	Status    string   `json:"status"`
	PIDs      []string `json:"pids"`
	CreatedAt int64    `json:"created_at"`
	Rootfs    string   `json:"rootfs"`
}

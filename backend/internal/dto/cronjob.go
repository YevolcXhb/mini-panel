package dto

type CronjobCreateRequest struct {
	Name    string `json:"name" binding:"required"`
	Spec    string `json:"spec" binding:"required"`
	Command string `json:"command" binding:"required"`
	Script  string `json:"script"`
}

type CronjobUpdateRequest struct {
	Name    string `json:"name"`
	Spec    string `json:"spec"`
	Command string `json:"command"`
	Script  string `json:"script"`
	Status  string `json:"status"`
}

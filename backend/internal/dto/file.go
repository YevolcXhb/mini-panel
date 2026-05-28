package dto

type FileListRequest struct {
	Path string `form:"path" binding:"required"`
}

type FileContentRequest struct {
	Path string `form:"path" binding:"required"`
}

type FileCreateRequest struct {
	Path    string `json:"path" binding:"required"`
	IsDir   bool   `json:"is_dir"`
	Content string `json:"content"`
}

type FileUpdateRequest struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type FileDeleteRequest struct {
	Path string `json:"path" binding:"required"`
}

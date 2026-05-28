package api

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type FileAPI struct {
	service *service.FileService
}

func NewFileAPI() *FileAPI {
	return &FileAPI{service: service.NewFileService()}
}

func (a *FileAPI) List(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		path = "/"
	}
	files, err := a.service.List(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: files})
}

func (a *FileAPI) GetContent(c *gin.Context) {
	path := c.Query("path")
	data, err := a.service.GetContent(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: string(data)})
}

func (a *FileAPI) Create(c *gin.Context) {
	var req dto.FileCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Create(req.Path, req.IsDir, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "created"})
}

func (a *FileAPI) Update(c *gin.Context) {
	var req dto.FileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Update(req.Path, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "updated"})
}

func (a *FileAPI) Delete(c *gin.Context) {
	var req dto.FileDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Delete(req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "deleted"})
}

func (a *FileAPI) Upload(c *gin.Context) {
	path := c.PostForm("path")
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	reader, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	defer reader.Close()

	target := filepath.Join(path, file.Filename)
	if err := a.service.Upload(target, reader); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "uploaded"})
}

func (a *FileAPI) Download(c *gin.Context) {
	path := c.Query("path")
	data, err := a.service.GetContent(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+filepath.Base(path))
	c.Data(http.StatusOK, "application/octet-stream", data)
}

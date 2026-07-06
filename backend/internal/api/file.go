package api

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

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
	// 移入回收站而非直接删除
	if err := a.service.MoveToRecycle(req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "moved to recycle bin"})
}

func (a *FileAPI) ForceDelete(c *gin.Context) {
	var req dto.FileDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Delete(req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "permanently deleted"})
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
	resolvedPath, err := a.service.ResolvePath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	if info.IsDir() {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "不能下载目录"})
		return
	}

	c.Header("Content-Disposition", "attachment; filename*=utf-8''"+url.PathEscape(info.Name()))
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

func (a *FileAPI) UploadMultiple(c *gin.Context) {
	path := c.PostForm("path")
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "no files uploaded"})
		return
	}
	successCount := 0
	for _, file := range files {
		reader, err := file.Open()
		if err != nil {
			continue
		}
		target := filepath.Join(path, file.Filename)
		if err := a.service.Upload(target, reader); err != nil {
			reader.Close()
			continue
		}
		reader.Close()
		successCount++
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: fmt.Sprintf("uploaded %d/%d files", successCount, len(files))})
}

func (a *FileAPI) DownloadZip(c *gin.Context) {
	path := c.Query("path")
	resolvedPath, err := a.service.ResolvePath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	info, err := os.Stat(resolvedPath)
	if err != nil || !info.IsDir() {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "path is not a directory"})
		return
	}
	zipName := info.Name() + ".zip"
	c.Header("Content-Disposition", "attachment; filename*=utf-8''"+url.PathEscape(zipName))
	c.Header("Content-Type", "application/zip")
	if err := a.service.CreateZip(path, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
}

func (a *FileAPI) Rename(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		NewName string `json:"new_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Rename(req.Path, req.NewName); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "renamed"})
}

func (a *FileAPI) Chmod(c *gin.Context) {
	var req struct {
		Path      string `json:"path" binding:"required"`
		Mode      string `json:"mode" binding:"required"`
		Recursive bool   `json:"recursive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	mode, err := strconv.ParseUint(req.Mode, 8, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "invalid mode"})
		return
	}
	if err := a.service.Chmod(req.Path, os.FileMode(mode), req.Recursive); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "chmod done"})
}

func (a *FileAPI) Compress(c *gin.Context) {
	var req struct {
		Paths  []string `json:"paths" binding:"required"`
		Output string   `json:"output" binding:"required"`
		Format string   `json:"format"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if req.Format == "" {
		req.Format = "zip"
	}
	if err := a.service.Compress(req.Paths, req.Output, req.Format); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "compressed"})
}

func (a *FileAPI) Extract(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		DestDir string `json:"dest_dir" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Extract(req.Path, req.DestDir); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "extracted"})
}

func (a *FileAPI) Copy(c *gin.Context) {
	var req struct {
		SrcPath  string `json:"src_path" binding:"required"`
		DestPath string `json:"dest_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.CopyFile(req.SrcPath, req.DestPath); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "copied"})
}

func (a *FileAPI) Move(c *gin.Context) {
	var req struct {
		SrcPath  string `json:"src_path" binding:"required"`
		DestPath string `json:"dest_path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.MoveFile(req.SrcPath, req.DestPath); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "moved"})
}

func (a *FileAPI) Search(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		path = "/"
	}
	search := c.Query("search")
	files, err := a.service.ListWithSearch(path, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: files})
}

func (a *FileAPI) ListRecycleBin(c *gin.Context) {
	files, err := a.service.ListRecycleBin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: files})
}

func (a *FileAPI) RestoreRecycle(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.RestoreFromRecycle(req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "restored"})
}

func (a *FileAPI) ClearRecycleBin(c *gin.Context) {
	if err := a.service.ClearRecycleBin(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "cleared"})
}

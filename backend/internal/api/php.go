package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/service"
)

type PhpAPI struct {
	svc *service.PhpService
}

func NewPhpAPI() *PhpAPI {
	return &PhpAPI{svc: service.NewPhpService()}
}

// GetVersions 获取所有 PHP 版本状态
func (a *PhpAPI) GetVersions(c *gin.Context) {
	versions := a.svc.GetInstalledVersions()
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: versions})
}

// InstallVersion 安装 PHP 版本
func (a *PhpAPI) InstallVersion(c *gin.Context) {
	var req model.PhpInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "请指定要安装的版本号"})
		return
	}
	if err := a.svc.InstallVersion(req.Version); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "PHP " + req.Version + " 安装成功"})
}

// RemoveVersion 卸载 PHP 版本
func (a *PhpAPI) RemoveVersion(c *gin.Context) {
	version := c.Param("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "请指定版本号"})
		return
	}
	if err := a.svc.RemoveVersion(version); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "PHP " + version + " 卸载成功"})
}

// StartFpm 启动 PHP-FPM
func (a *PhpAPI) StartFpm(c *gin.Context) {
	version := c.Param("version")
	if err := a.svc.StartFpm(version); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "PHP-FPM " + version + " 启动成功"})
}

// StopFpm 停止 PHP-FPM
func (a *PhpAPI) StopFpm(c *gin.Context) {
	version := c.Param("version")
	if err := a.svc.StopFpm(version); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "PHP-FPM " + version + " 停止成功"})
}

// RestartFpm 重启 PHP-FPM
func (a *PhpAPI) RestartFpm(c *gin.Context) {
	version := c.Param("version")
	if err := a.svc.RestartFpm(version); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "PHP-FPM " + version + " 重启成功"})
}

// GetExtensions 获取扩展列表
func (a *PhpAPI) GetExtensions(c *gin.Context) {
	version := c.Param("version")
	exts, err := a.svc.GetExtensions(version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: exts})
}

// InstallExtension 安装扩展
func (a *PhpAPI) InstallExtension(c *gin.Context) {
	version := c.Param("version")
	var req model.PhpExtensionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "请指定扩展名"})
		return
	}
	if err := a.svc.InstallExtension(version, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "扩展 " + req.Name + " 安装成功"})
}

// RemoveExtension 卸载扩展
func (a *PhpAPI) RemoveExtension(c *gin.Context) {
	version := c.Param("version")
	name := c.Param("name")
	if strings.TrimSpace(name) == "" {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "请指定扩展名"})
		return
	}
	if err := a.svc.RemoveExtension(version, name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "扩展 " + name + " 卸载成功"})
}

// GetPhpIni 获取 PHP 配置
func (a *PhpAPI) GetPhpIni(c *gin.Context) {
	version := c.Param("version")
	items, err := a.svc.GetPhpIni(version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

// UpdatePhpIni 修改 PHP 配置
func (a *PhpAPI) UpdatePhpIni(c *gin.Context) {
	version := c.Param("version")
	var items []model.PhpConfigItem
	if err := c.ShouldBindJSON(&items); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "配置项格式错误"})
		return
	}
	if err := a.svc.UpdatePhpIni(version, items); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "配置已更新"})
}

// GetFpmSocket 获取 PHP-FPM socket 路径
func (a *PhpAPI) GetFpmSocket(c *gin.Context) {
	version := c.Param("version")
	socket := a.svc.GetFpmSocket(version)
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: socket})
}
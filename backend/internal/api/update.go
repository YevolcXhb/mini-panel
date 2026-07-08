package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/service"
)

// UpdateAPI 面板更新相关 API
type UpdateAPI struct {
	service *service.UpdateService
}

func NewUpdateAPI() *UpdateAPI {
	return &UpdateAPI{service: service.NewUpdateService()}
}

// Check 检查最新版本
// GET /api/v1/update/check
func (a *UpdateAPI) Check(c *gin.Context) {
	info, err := a.service.CheckUpdate()
	if err != nil {
		global.LOG.Warnf("[UpdateAPI] 检查更新失败: %v", err)
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	global.LOG.Infof("[UpdateAPI] 检查更新: 当前=%s, 最新=%s, 有更新=%v, 来源=%s",
		info.CurrentVersion, info.LatestVersion, info.HasUpdate, info.Source)
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: info})
}

// Apply 触发更新（下载并执行 install.sh）
// POST /api/v1/update/apply
func (a *UpdateAPI) Apply(c *gin.Context) {
	result, err := a.service.ApplyUpdate()
	if err != nil {
		global.LOG.Errorf("[UpdateAPI] 触发更新失败: %v", err)
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: result})
}

// Status 查询当前更新任务状态
// GET /api/v1/update/status
func (a *UpdateAPI) Status(c *gin.Context) {
	status := a.service.GetStatus()
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: status})
}

// Log 获取更新日志（最后 N 行，默认 100）
// GET /api/v1/update/log?tail=100
func (a *UpdateAPI) Log(c *gin.Context) {
	tail := 100
	if s := c.Query("tail"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			tail = n
		}
	}
	log, err := a.service.GetUpdateLog(tail)
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: gin.H{"log": log}})
}

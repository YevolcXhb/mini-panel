package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type DashboardAPI struct {
	service *service.DashboardService
}

func NewDashboardAPI() *DashboardAPI {
	return &DashboardAPI{service: service.NewDashboardService()}
}

func (a *DashboardAPI) GetInfo(c *gin.Context) {
	info, err := a.service.GetSystemInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	cpu, err := a.service.GetCPUInfo()
	if err != nil {
		cpu = nil
	}
	mem, err := a.service.GetMemoryInfo()
	if err != nil {
		mem = nil
	}
	disk, err := a.service.GetDiskInfo()
	if err != nil {
		disk = nil
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: gin.H{
		"system": info,
		"cpu":    cpu,
		"memory": mem,
		"disk":   disk,
	}})
}

func (a *DashboardAPI) GetMonitor(c *gin.Context) {
	mode := c.DefaultQuery("mode", "chroot")
	data, err := a.service.GetMonitor(mode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: data})
}

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type SettingAPI struct {
	service *service.SettingService
}

func NewSettingAPI() *SettingAPI {
	return &SettingAPI{service: service.NewSettingService()}
}

func (a *SettingAPI) Get(c *gin.Context) {
	items, err := a.service.List()
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

func (a *SettingAPI) Update(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	for k, v := range req {
		if err := a.service.Set(k, v); err != nil {
			c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "updated"})
}

func (a *SettingAPI) Reset(c *gin.Context) {
	if err := a.service.InitDefaults(); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "reset"})
}

func (a *SettingAPI) ClearData(c *gin.Context) {
	if err := a.service.ClearData(); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "all data cleared, please re-login with admin/123456"})
}

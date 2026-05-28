package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type AppAPI struct {
	service *service.AppService
}

func NewAppAPI() *AppAPI {
	return &AppAPI{service: service.NewAppService()}
}

func (a *AppAPI) List(c *gin.Context) {
	apps, err := a.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: apps})
}

func (a *AppAPI) Install(c *gin.Context) {
	var req dto.AppInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	inst, err := a.service.Install(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: inst})
}

func (a *AppAPI) Uninstall(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	fmt.Sscanf(idStr, "%d", &id)
	if err := a.service.Uninstall(id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "uninstalled"})
}

func (a *AppAPI) Installed(c *gin.Context) {
	items, err := a.service.Installed()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

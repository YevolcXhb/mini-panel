package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	systemutil "github.com/minipanel/minipanel/internal/utils/system"
)

type SystemAPI struct{}

func NewSystemAPI() *SystemAPI {
	return &SystemAPI{}
}

func (a *SystemAPI) CheckServices(c *gin.Context) {
	services := systemutil.GetAllServices()
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: services})
}

func (a *SystemAPI) InstallService(c *gin.Context) {
	name := c.Param("name")
	if err := systemutil.InstallService(name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: name + " installed successfully"})
}

func (a *SystemAPI) StartService(c *gin.Context) {
	name := c.Param("name")
	if err := systemutil.StartService(name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: name + " started successfully"})
}

func (a *SystemAPI) StopService(c *gin.Context) {
	name := c.Param("name")
	if err := systemutil.StopService(name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: name + " stopped successfully"})
}

func (a *SystemAPI) RestartService(c *gin.Context) {
	name := c.Param("name")
	if err := systemutil.RestartService(name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: name + " restarted successfully"})
}

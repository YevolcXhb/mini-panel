package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type MonitorAPI struct {
	service *service.MonitorService
}

func NewMonitorAPI() *MonitorAPI {
	return &MonitorAPI{service: service.NewMonitorService()}
}

func (a *MonitorAPI) List(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1440"))
		items, err := a.service.List(limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
	}

	func (a *MonitorAPI) GetRealtime(c *gin.Context) {
		metrics, err := a.service.GetRealtime()
		if err != nil {
			c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, dto.Response{Code: 200, Data: metrics})
	}

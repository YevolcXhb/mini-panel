package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type ContainerAPI struct {
	service *service.ContainerService
}

func NewContainerAPI() *ContainerAPI {
	return &ContainerAPI{service: service.NewContainerService()}
}

func (a *ContainerAPI) List(c *gin.Context) {
	items, err := a.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

func (a *ContainerAPI) Inspect(c *gin.Context) {
	name := c.Param("name")
	item, err := a.service.Inspect(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: item})
}

func (a *ContainerAPI) Create(c *gin.Context) {
	var req dto.ContainerCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Create(req); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "created"})
}

func (a *ContainerAPI) Start(c *gin.Context) {
	name := c.Param("name")
	if err := a.service.Start(name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "started"})
}

func (a *ContainerAPI) Stop(c *gin.Context) {
	name := c.Param("name")
	if err := a.service.Stop(name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "stopped"})
}

func (a *ContainerAPI) Remove(c *gin.Context) {
	name := c.Param("name")
	if err := a.service.Remove(name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "removed"})
}

func (a *ContainerAPI) Logs(c *gin.Context) {
	name := c.Param("name")
	tailStr := c.DefaultQuery("tail", "100")
	tail, _ := strconv.Atoi(tailStr)
	logs, err := a.service.Logs(name, tail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: logs})
}

func (a *ContainerAPI) ListFiles(c *gin.Context) {
	name := c.Param("name")
	path := c.Query("path")
	if path == "" {
		path = "/"
	}
	files, err := a.service.ListFiles(name, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: files})
}

func (a *ContainerAPI) Pull(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
		Name  string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Pull(req.Image, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "pulled"})
}

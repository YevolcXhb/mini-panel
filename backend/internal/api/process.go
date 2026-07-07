package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type ProcessAPI struct {
	service *service.ProcessService
}

func NewProcessAPI() *ProcessAPI {
	return &ProcessAPI{service: service.NewProcessService()}
}

func (a *ProcessAPI) List(c *gin.Context) {
	procs, err := a.service.List()
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: procs})
}

func (a *ProcessAPI) Kill(c *gin.Context) {
	var req struct {
		Pid   string `json:"pid"`
		Force bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: "invalid request"})
		return
	}
	var err error
	if req.Force {
		err = a.service.KillForce(req.Pid)
	} else {
		err = a.service.Kill(req.Pid)
	}
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "killed"})
}

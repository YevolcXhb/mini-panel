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
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: procs})
}

func (a *ProcessAPI) Kill(c *gin.Context) {
	pid := c.PostForm("pid")
	force := c.PostForm("force") == "true"
	var err error
	if force {
		err = a.service.KillForce(pid)
	} else {
		err = a.service.Kill(pid)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "killed"})
}

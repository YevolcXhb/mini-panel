package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type CronjobAPI struct {
	service *service.CronjobService
}

func NewCronjobAPI() *CronjobAPI {
	return &CronjobAPI{service: service.NewCronjobService()}
}

func (a *CronjobAPI) List(c *gin.Context) {
	items, err := a.service.List()
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

func (a *CronjobAPI) Create(c *gin.Context) {
	var req dto.CronjobCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	item, err := a.service.Create(req)
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: item})
}

func (a *CronjobAPI) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)
	var req dto.CronjobUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	item, err := a.service.Update(uint(id), req)
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: item})
}

func (a *CronjobAPI) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)
	if err := a.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "deleted"})
}

func (a *CronjobAPI) Run(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)
	if err := a.service.Run(uint(id)); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "executed"})
}

package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/service"
)

type WebsiteAPI struct {
	service *service.WebsiteService
}

func NewWebsiteAPI() *WebsiteAPI {
	return &WebsiteAPI{service: service.NewWebsiteService()}
}

func (a *WebsiteAPI) Create(c *gin.Context) {
	var w model.Website
	if err := c.ShouldBindJSON(&w); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.Create(&w); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "created"})
}

func (a *WebsiteAPI) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var w model.Website
	if err := c.ShouldBindJSON(&w); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	w.ID = uint(id)
	// 外部站点（id=0）当作新建处理
	if id == 0 {
		if err := a.service.Create(&w); err != nil {
			c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "created"})
		return
	}
	if err := a.service.Update(&w); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "updated"})
}

func (a *WebsiteAPI) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id == 0 {
		// 外部站点：使用 query 参数 domain+port
		domain := c.Query("domain")
		portStr := c.Query("port")
		port, _ := strconv.Atoi(portStr)
		if domain == "" || port == 0 {
			c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "缺少 domain/port 参数"})
			return
		}
		if err := a.service.DeleteExternal(domain, port); err != nil {
			c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "deleted"})
		return
	}
	if err := a.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "deleted"})
}

func (a *WebsiteAPI) List(c *gin.Context) {
	items, err := a.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

func (a *WebsiteAPI) GetByID(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	w, err := a.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: w})
}

func (a *WebsiteAPI) Toggle(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if id == 0 {
		// 外部站点：用 domain+port
		domain := c.Query("domain")
		portStr := c.Query("port")
		port, _ := strconv.Atoi(portStr)
		if domain == "" || port == 0 {
			c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "缺少 domain/port 参数"})
			return
		}
		if err := a.service.ToggleExternal(domain, port, req.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
			return
		}
		c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "toggled"})
		return
	}
	if err := a.service.ToggleEnable(uint(id), req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "toggled"})
}

func (a *WebsiteAPI) ReloadNginx(c *gin.Context) {
	if err := a.service.ReloadNginx(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "nginx reloaded"})
}

func (a *WebsiteAPI) GetNginxStatus(c *gin.Context) {
	status, err := a.service.GetNginxStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: status})
}

func (a *WebsiteAPI) StartNginx(c *gin.Context) {
	if err := a.service.StartNginx(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "nginx started"})
}

func (a *WebsiteAPI) StopNginx(c *gin.Context) {
	if err := a.service.StopNginx(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "nginx stopped"})
}

func (a *WebsiteAPI) RestartNginx(c *gin.Context) {
	if err := a.service.RestartNginx(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "nginx restarted"})
}

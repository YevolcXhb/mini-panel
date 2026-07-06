package api

import (
	"net/http"
	"strconv"
	"strings"

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
	var req dto.WebsiteCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	// 联动建库走新路径，否则走旧 Create 保持向下兼容
	if req.DBCreate {
		if err := a.service.CreateWithDB(&req); err != nil {
			c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
			return
		}
	} else {
		if err := a.service.Create(&req.Website); err != nil {
			c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: req.Website, Message: "created"})
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
	// 读取 cascade_db 参数（默认 true；"false"/"0"/"no" 视为 false）
	cascadeStr := strings.ToLower(c.DefaultQuery("cascade_db", "true"))
	cascadeDB := !(cascadeStr == "false" || cascadeStr == "0" || cascadeStr == "no" || cascadeStr == "")
	if err := a.service.DeleteWithCascade(uint(id), cascadeDB); err != nil {
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

// ListDatabasesByWebsite 查询网站关联的数据库
func (a *WebsiteAPI) ListDatabasesByWebsite(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "invalid id"})
		return
	}
	items, err := a.service.ListDatabasesByWebsiteID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

// ListWebsitesByDB 查询数据库实例被哪些网站引用
func (a *WebsiteAPI) ListWebsitesByDB(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "invalid id"})
		return
	}
	items, err := a.service.ListWebsitesByInstanceID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
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

func (a *WebsiteAPI) GetAccessLogs(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	w, err := a.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, dto.Response{Code: 404, Message: "website not found"})
		return
	}
	var filters service.AccessLogFilter
	filters.Date = c.Query("date")
	filters.IP = c.Query("ip")
	filters.StatusCode = c.Query("status_code")
	filters.URL = c.Query("url")
	if page, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		filters.Page = page
	}
	if pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "50")); err == nil {
		filters.PageSize = pageSize
	}
	entries, total, err := a.service.ParseAccessLogs(w, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: gin.H{"entries": entries, "total": total}})
}

func (a *WebsiteAPI) GetTrafficStats(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	w, err := a.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, dto.Response{Code: 404, Message: "website not found"})
		return
	}
	period := c.DefaultQuery("period", "24h")
	stats, err := a.service.GetTrafficStats(w, period)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: stats})
}

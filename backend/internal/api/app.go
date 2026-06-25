package api

import (
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type iconCache struct {
	data      []byte
	etag      string
	fetchedAt time.Time
}

var (
	iconCacheMap = make(map[string]*iconCache)
	iconCacheMu  sync.RWMutex
)

type AppAPI struct {
	service *service.AppService
}

func NewAppAPI() *AppAPI {
	return &AppAPI{service: service.NewAppService()}
}

func (a *AppAPI) List(c *gin.Context) {
	category := c.Query("category")
	apps, err := a.service.List(category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: apps})
}

func (a *AppAPI) Search(c *gin.Context) {
	keyword := c.Query("q")
	apps, err := a.service.Search(keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: apps})
}

func (a *AppAPI) Detail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "invalid id"})
		return
	}
	app, details, err := a.service.GetWithDetails(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, dto.Response{Code: 404, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: gin.H{
		"app":     app,
		"details": details,
	}})
}

func (a *AppAPI) Installed(c *gin.Context) {
	items, err := a.service.Installed()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

func (a *AppAPI) Install(c *gin.Context) {
	var req dto.AppInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	inst, err := a.service.Install(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error(), Data: inst})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: inst})
}

func (a *AppAPI) Uninstall(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "invalid id"})
		return
	}
	if err := a.service.Uninstall(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "uninstalled"})
}

func (a *AppAPI) ClearHistory(c *gin.Context) {
	if err := a.service.ClearHistory(); err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "cleared"})
}

func (a *AppAPI) Sync(c *gin.Context) {
	var req dto.AppSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	if err := a.service.SyncFromRemote(req.SourceID); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "synced"})
}

func (a *AppAPI) Sources(c *gin.Context) {
	items, err := a.service.ListSources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

func (a *AppAPI) AddSource(c *gin.Context) {
	var req dto.AppSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	source, err := a.service.AddSource(req.Name, req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: source})
}

func (a *AppAPI) RemoveSource(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: "invalid id"})
		return
	}
	if err := a.service.RemoveSource(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "removed"})
}

func (a *AppAPI) Icon(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	iconURL, err := a.service.GetIconURL(key)
	if err != nil || iconURL == "" {
		c.Status(http.StatusNoContent)
		return
	}

	iconCacheMu.RLock()
	cached, ok := iconCacheMap[key]
	iconCacheMu.RUnlock()

	if ok && time.Since(cached.fetchedAt) < 24*time.Hour {
		ifNoneMatch := c.GetHeader("If-None-Match")
		if ifNoneMatch != "" && ifNoneMatch == cached.etag {
			c.Status(http.StatusNotModified)
			return
		}
		if cached.etag != "" {
			c.Header("ETag", cached.etag)
		}
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, "image/png", cached.data)
		return
	}

	resp, err := http.Get(iconURL)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.Status(http.StatusBadGateway)
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		c.Status(http.StatusBadGateway)
		return
	}

	etag := resp.Header.Get("ETag")

	iconCacheMu.Lock()
	iconCacheMap[key] = &iconCache{
		data:      data,
		etag:      etag,
		fetchedAt: time.Now(),
	}
	iconCacheMu.Unlock()

	if etag != "" {
		c.Header("ETag", etag)
	}
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/png", data)
}

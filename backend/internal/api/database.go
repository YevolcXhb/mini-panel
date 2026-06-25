package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/service"
)

type DatabaseAPI struct {
	service *service.DatabaseService
}

func NewDatabaseAPI() *DatabaseAPI {
	return &DatabaseAPI{service: service.NewDatabaseService()}
}

func (h *DatabaseAPI) Create(c *gin.Context) {
	var req model.DatabaseInstance
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := h.service.Create(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Database instance created"})
}

func (h *DatabaseAPI) List(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

func (h *DatabaseAPI) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	var req model.DatabaseInstance
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	req.ID = uint(id)
	if err := h.service.Update(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Database instance updated"})
}

func (h *DatabaseAPI) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	if err := h.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Database instance deleted"})
}

func (h *DatabaseAPI) TestConnection(c *gin.Context) {
	var req model.DatabaseInstance
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	msg, err := h.service.TestConnection(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": msg})
}

func (h *DatabaseAPI) ListDatabases(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	item, err := h.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "instance not found"})
		return
	}
	dbs, err := h.service.ListDatabases(item)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": dbs})
}

func (h *DatabaseAPI) ListTables(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	item, err := h.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "instance not found"})
		return
	}
	tables, err := h.service.ListTables(item)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": tables})
}

func (h *DatabaseAPI) CreateDatabase(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	var req struct {
		DBName string `json:"db_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	item, err := h.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "instance not found"})
		return
	}
	if err := h.service.CreateDatabase(item, req.DBName); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Database created successfully"})
}

func (h *DatabaseAPI) CreateUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		PrivDB   string `json:"priv_db"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	item, err := h.service.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "instance not found"})
		return
	}
	if err := h.service.CreateUser(item, req.Username, req.Password, req.PrivDB); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "User created successfully"})
}

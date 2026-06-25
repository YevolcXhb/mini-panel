package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/service"
)

type BackupAPI struct {
	service *service.BackupService
}

func NewBackupAPI() *BackupAPI {
	return &BackupAPI{service: service.NewBackupService()}
}

func (h *BackupAPI) ListTasks(c *gin.Context) {
	items, err := h.service.ListTasks()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

func (h *BackupAPI) CreateTask(c *gin.Context) {
	var item model.BackupTask
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := h.service.Create(&item); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Task created", "data": item})
}

func (h *BackupAPI) UpdateTask(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var item model.BackupTask
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	item.ID = uint(id)
	if err := h.service.UpdateTask(&item); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Task updated"})
}

func (h *BackupAPI) DeleteTask(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.service.DeleteTask(uint(id)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Task deleted"})
}

func (h *BackupAPI) RunBackup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	record, err := h.service.RunBackup(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Backup started", "data": record})
}

func (h *BackupAPI) ListRecords(c *gin.Context) {
	taskID, _ := strconv.Atoi(c.Query("task_id"))
	items, err := h.service.ListRecords(uint(taskID))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

func (h *BackupAPI) DeleteRecord(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.service.DeleteRecord(uint(id)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Record deleted"})
}

func (h *BackupAPI) RestoreBackup(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.service.RestoreBackup(uint(id)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Backup restored successfully"})
}

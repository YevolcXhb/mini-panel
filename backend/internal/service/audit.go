package service

import (
	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
)

type AuditService struct {
	repo *repository.AuditLogRepository
}

func NewAuditService() *AuditService {
	return &AuditService{
		repo: repository.NewAuditLogRepository(global.DB),
	}
}

func (s *AuditService) Log(username, action, resource, detail, ip string, success bool) {
	_ = s.repo.Create(&model.AuditLog{
		Username: username,
		Action:   action,
		Resource: resource,
		Detail:   detail,
		IP:       ip,
		Success:  success,
	})
}

func (s *AuditService) LogFromContext(c *gin.Context, action, resource, detail string, success bool) {
	username, _ := c.Get("user")
	userStr, _ := username.(string)
	ip := c.ClientIP()
	s.Log(userStr, action, resource, detail, ip, success)
}

func (s *AuditService) List(limit int) ([]model.AuditLog, error) {
	return s.repo.List(limit)
}

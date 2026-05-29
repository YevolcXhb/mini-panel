package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type AuditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(log *model.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *AuditLogRepository) List(limit int) ([]model.AuditLog, error) {
	var logs []model.AuditLog
	err := r.db.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

func (r *AuditLogRepository) ListByUser(username string, limit int) ([]model.AuditLog, error) {
	var logs []model.AuditLog
	err := r.db.Where("username = ?", username).Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

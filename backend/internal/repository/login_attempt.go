package repository

import (
	"time"

	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type LoginAttemptRepository struct {
	db *gorm.DB
}

func NewLoginAttemptRepository(db *gorm.DB) *LoginAttemptRepository {
	return &LoginAttemptRepository{db: db}
}

func (r *LoginAttemptRepository) Create(attempt *model.LoginAttempt) error {
	return r.db.Create(attempt).Error
}

func (r *LoginAttemptRepository) CountRecentFailures(username, ip string, since time.Time) (int64, error) {
	var count int64
	err := r.db.Model(&model.LoginAttempt{}).
		Where("(username = ? OR ip = ?) AND success = ? AND created_at >= ?", username, ip, false, since).
		Count(&count).Error
	return count, err
}

func (r *LoginAttemptRepository) GetActiveLock(username, ip string) (*model.LoginAttempt, error) {
	var attempt model.LoginAttempt
	err := r.db.Where("(username = ? OR ip = ?) AND locked_until IS NOT NULL AND locked_until > ?", username, ip, time.Now()).
		Order("locked_until DESC").
		First(&attempt).Error
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func (r *LoginAttemptRepository) ClearOld(before time.Time) error {
	return r.db.Where("created_at < ?", before).Delete(&model.LoginAttempt{}).Error
}

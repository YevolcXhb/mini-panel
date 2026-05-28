package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type SettingRepository struct {
	db *gorm.DB
}

func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

func (r *SettingRepository) Get(key string) (*model.Setting, error) {
	var s model.Setting
	if err := r.db.Where("key = ?", key).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SettingRepository) Set(key, value string) error {
	var s model.Setting
	if err := r.db.Where("key = ?", key).First(&s).Error; err != nil {
		return r.db.Create(&model.Setting{Key: key, Value: value}).Error
	}
	s.Value = value
	return r.db.Save(&s).Error
}

func (r *SettingRepository) List() ([]model.Setting, error) {
	var items []model.Setting
	err := r.db.Find(&items).Error
	return items, err
}

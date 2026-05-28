package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type CronjobRepository struct {
	db *gorm.DB
}

func NewCronjobRepository(db *gorm.DB) *CronjobRepository {
	return &CronjobRepository{db: db}
}

func (r *CronjobRepository) List() ([]model.Cronjob, error) {
	var items []model.Cronjob
	err := r.db.Find(&items).Error
	return items, err
}

func (r *CronjobRepository) GetByID(id uint) (*model.Cronjob, error) {
	var item model.Cronjob
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *CronjobRepository) Create(item *model.Cronjob) error {
	return r.db.Create(item).Error
}

func (r *CronjobRepository) Update(item *model.Cronjob) error {
	return r.db.Save(item).Error
}

func (r *CronjobRepository) Delete(id uint) error {
	return r.db.Delete(&model.Cronjob{}, id).Error
}

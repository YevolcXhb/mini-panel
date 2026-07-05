package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type WebsiteRepository struct {
	db *gorm.DB
}

func NewWebsiteRepository(db *gorm.DB) *WebsiteRepository {
	return &WebsiteRepository{db: db}
}

func (r *WebsiteRepository) Create(w *model.Website) error {
	return r.db.Create(w).Error
}

func (r *WebsiteRepository) Update(w *model.Website) error {
	return r.db.Save(w).Error
}

func (r *WebsiteRepository) Delete(id uint) error {
	return r.db.Delete(&model.Website{}, id).Error
}

func (r *WebsiteRepository) GetByID(id uint) (*model.Website, error) {
	var w model.Website
	err := r.db.First(&w, id).Error
	return &w, err
}

func (r *WebsiteRepository) List() ([]model.Website, error) {
	var items []model.Website
	err := r.db.Order("id DESC").Find(&items).Error
	return items, err
}

// GetByDomainPort 通过域名+端口查找
func (r *WebsiteRepository) GetByDomainPort(domain string, port int) (*model.Website, error) {
	var w model.Website
	err := r.db.Where("domain = ? AND port = ?", domain, port).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// DeleteByDomainPort 通过域名+端口删除
func (r *WebsiteRepository) DeleteByDomainPort(domain string, port int) error {
	return r.db.Where("domain = ? AND port = ?", domain, port).Delete(&model.Website{}).Error
}

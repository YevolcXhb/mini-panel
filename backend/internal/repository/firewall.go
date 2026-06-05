package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type FirewallRepository struct {
	db *gorm.DB
}

func NewFirewallRepository(db *gorm.DB) *FirewallRepository {
	return &FirewallRepository{db: db}
}

func (r *FirewallRepository) Create(item *model.FirewallRule) error {
	return r.db.Create(item).Error
}

func (r *FirewallRepository) List() ([]model.FirewallRule, error) {
	var items []model.FirewallRule
	err := r.db.Order("id DESC").Find(&items).Error
	return items, err
}

func (r *FirewallRepository) GetByID(id uint) (*model.FirewallRule, error) {
	var item model.FirewallRule
	err := r.db.First(&item, id).Error
	return &item, err
}

func (r *FirewallRepository) Update(item *model.FirewallRule) error {
	return r.db.Save(item).Error
}

func (r *FirewallRepository) Delete(id uint) error {
	return r.db.Delete(&model.FirewallRule{}, id).Error
}

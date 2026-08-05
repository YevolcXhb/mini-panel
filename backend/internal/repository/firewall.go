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

// ListDeleted 查询已软删除的规则，供面板回收站恢复使用
func (r *FirewallRepository) ListDeleted() ([]model.FirewallRule, error) {
	var items []model.FirewallRule
	err := r.db.Unscoped().Where("deleted_at IS NOT NULL").Order("id DESC").Find(&items).Error
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

// Restore 恢复被软删除的规则
func (r *FirewallRepository) Restore(id uint) error {
	return r.db.Unscoped().Model(&model.FirewallRule{}).
		Where("id = ?", id).
		Update("deleted_at", nil).Error
}

// ClearDeleted 永久清空所有已软删除的规则，返回删除条数
func (r *FirewallRepository) ClearDeleted() (int64, error) {
	res := r.db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&model.FirewallRule{})
	return res.RowsAffected, res.Error
}

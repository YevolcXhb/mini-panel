package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type DatabaseRepository struct {
	db *gorm.DB
}

func NewDatabaseRepository(db *gorm.DB) *DatabaseRepository {
	return &DatabaseRepository{db: db}
}

func (r *DatabaseRepository) Create(item *model.DatabaseInstance) error {
	return r.db.Create(item).Error
}

func (r *DatabaseRepository) List() ([]model.DatabaseInstance, error) {
	var items []model.DatabaseInstance
	err := r.db.Order("id DESC").Find(&items).Error
	return items, err
}

func (r *DatabaseRepository) GetByID(id uint) (*model.DatabaseInstance, error) {
	var item model.DatabaseInstance
	err := r.db.First(&item, id).Error
	return &item, err
}

func (r *DatabaseRepository) GetByName(name string) (*model.DatabaseInstance, error) {
	var item model.DatabaseInstance
	err := r.db.Where("name = ?", name).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// GetByNameWithUnscoped 查找同名记录（包括软删除的）
// 用于检测软删除记录是否存在，避免 uniqueIndex 冲突
func (r *DatabaseRepository) GetByNameWithUnscoped(name string) (*model.DatabaseInstance, error) {
	var item model.DatabaseInstance
	err := r.db.Unscoped().Where("name = ?", name).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// RestoreSoftDeleted 物理删除软删除的同名记录，为新建腾出 uniqueIndex 槽位
func (r *DatabaseRepository) RestoreSoftDeleted(name string) error {
	return r.db.Unscoped().Where("name = ?", name).Delete(&model.DatabaseInstance{}).Error
}

func (r *DatabaseRepository) Update(item *model.DatabaseInstance) error {
	return r.db.Save(item).Error
}

func (r *DatabaseRepository) Delete(id uint) error {
	return r.db.Delete(&model.DatabaseInstance{}, id).Error
}

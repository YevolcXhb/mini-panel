package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type WebsiteDatabaseRepository struct {
	db *gorm.DB
}

func NewWebsiteDatabaseRepository(db *gorm.DB) *WebsiteDatabaseRepository {
	return &WebsiteDatabaseRepository{db: db}
}

func (r *WebsiteDatabaseRepository) Create(item *model.WebsiteDatabase) error {
	return r.db.Create(item).Error
}

func (r *WebsiteDatabaseRepository) GetByWebsiteID(websiteID uint) ([]model.WebsiteDatabase, error) {
	var items []model.WebsiteDatabase
	err := r.db.Where("website_id = ?", websiteID).Find(&items).Error
	return items, err
}

func (r *WebsiteDatabaseRepository) GetByInstanceID(instanceID uint) ([]model.WebsiteDatabase, error) {
	var items []model.WebsiteDatabase
	err := r.db.Where("db_instance_id = ?", instanceID).Find(&items).Error
	return items, err
}

func (r *WebsiteDatabaseRepository) GetByInstanceAndDBName(instanceID uint, dbName string) (*model.WebsiteDatabase, error) {
	var item model.WebsiteDatabase
	err := r.db.Where("db_instance_id = ? AND db_name = ?", instanceID, dbName).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *WebsiteDatabaseRepository) GetByInstanceAndUsername(instanceID uint, username string) (*model.WebsiteDatabase, error) {
	var item model.WebsiteDatabase
	err := r.db.Where("db_instance_id = ? AND db_username = ?", instanceID, username).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *WebsiteDatabaseRepository) Update(item *model.WebsiteDatabase) error {
	return r.db.Save(item).Error
}

func (r *WebsiteDatabaseRepository) Delete(id uint) error {
	return r.db.Delete(&model.WebsiteDatabase{}, id).Error
}

func (r *WebsiteDatabaseRepository) DeleteByWebsiteID(websiteID uint) error {
	return r.db.Where("website_id = ?", websiteID).Delete(&model.WebsiteDatabase{}).Error
}

func (r *WebsiteDatabaseRepository) CountByInstanceID(instanceID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.WebsiteDatabase{}).Where("db_instance_id = ?", instanceID).Count(&count).Error
	return count, err
}

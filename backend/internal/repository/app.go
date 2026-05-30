package repository

import (
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

type AppRepository struct {
	db *gorm.DB
}

func NewAppRepository(db *gorm.DB) *AppRepository {
	return &AppRepository{db: db}
}

func (r *AppRepository) List() ([]model.App, error) {
	var apps []model.App
	err := r.db.Where("status = ?", "active").Find(&apps).Error
	return apps, err
}

func (r *AppRepository) ListByCategory(category string) ([]model.App, error) {
	var apps []model.App
	q := r.db.Where("status = ?", "active")
	if category != "" && category != "all" {
		q = q.Where("category = ?", category)
	}
	err := q.Find(&apps).Error
	return apps, err
}

func (r *AppRepository) Search(keyword string) ([]model.App, error) {
	var apps []model.App
	err := r.db.Where("status = ? AND (name LIKE ? OR key LIKE ? OR description LIKE ?)", "active", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%").Find(&apps).Error
	return apps, err
}

func (r *AppRepository) GetByID(id uint) (*model.App, error) {
	var app model.App
	if err := r.db.First(&app, id).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *AppRepository) GetByKey(key string) (*model.App, error) {
	var app model.App
	if err := r.db.Where("`key` = ?", key).First(&app).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *AppRepository) Create(app *model.App) error {
	return r.db.Create(app).Error
}

func (r *AppRepository) Update(app *model.App) error {
	return r.db.Save(app).Error
}

func (r *AppRepository) Delete(id uint) error {
	return r.db.Delete(&model.App{}, id).Error
}

func (r *AppRepository) Clear() error {
	return r.db.Where("resource = ?", "remote").Delete(&model.App{}).Error
}

type AppDetailRepository struct {
	db *gorm.DB
}

func NewAppDetailRepository(db *gorm.DB) *AppDetailRepository {
	return &AppDetailRepository{db: db}
}

func (r *AppDetailRepository) ListByAppID(appID uint) ([]model.AppDetail, error) {
	var details []model.AppDetail
	err := r.db.Where("app_id = ? AND status = ?", appID, "active").Find(&details).Error
	return details, err
}

func (r *AppDetailRepository) GetByID(id uint) (*model.AppDetail, error) {
	var d model.AppDetail
	if err := r.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *AppDetailRepository) Create(d *model.AppDetail) error {
	return r.db.Create(d).Error
}

func (r *AppDetailRepository) Update(d *model.AppDetail) error {
	return r.db.Save(d).Error
}

func (r *AppDetailRepository) DeleteByAppID(appID uint) error {
	return r.db.Where("app_id = ?", appID).Delete(&model.AppDetail{}).Error
}

type AppInstallRepository struct {
	db *gorm.DB
}

func NewAppInstallRepository(db *gorm.DB) *AppInstallRepository {
	return &AppInstallRepository{db: db}
}

func (r *AppInstallRepository) List() ([]model.AppInstall, error) {
	var items []model.AppInstall
	err := r.db.Find(&items).Error
	return items, err
}

func (r *AppInstallRepository) GetByID(id uint) (*model.AppInstall, error) {
	var item model.AppInstall
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AppInstallRepository) GetByName(name string) (*model.AppInstall, error) {
	var item model.AppInstall
	if err := r.db.Where("name = ?", name).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AppInstallRepository) Create(item *model.AppInstall) error {
	return r.db.Create(item).Error
}

func (r *AppInstallRepository) Update(item *model.AppInstall) error {
	return r.db.Save(item).Error
}

func (r *AppInstallRepository) Delete(id uint) error {
	return r.db.Delete(&model.AppInstall{}, id).Error
}

type AppSourceRepository struct {
	db *gorm.DB
}

func NewAppSourceRepository(db *gorm.DB) *AppSourceRepository {
	return &AppSourceRepository{db: db}
}

func (r *AppSourceRepository) List() ([]model.AppSource, error) {
	var items []model.AppSource
	err := r.db.Find(&items).Error
	return items, err
}

func (r *AppSourceRepository) GetByID(id uint) (*model.AppSource, error) {
	var item model.AppSource
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *AppSourceRepository) Create(item *model.AppSource) error {
	return r.db.Create(item).Error
}

func (r *AppSourceRepository) Update(item *model.AppSource) error {
	return r.db.Save(item).Error
}

func (r *AppSourceRepository) Delete(id uint) error {
	return r.db.Delete(&model.AppSource{}, id).Error
}

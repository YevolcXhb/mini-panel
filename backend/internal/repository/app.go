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
	err := r.db.Find(&apps).Error
	return apps, err
}

func (r *AppRepository) GetByID(id uint) (*model.App, error) {
	var app model.App
	if err := r.db.First(&app, id).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *AppRepository) Create(app *model.App) error {
	return r.db.Create(app).Error
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

func (r *AppInstallRepository) Create(item *model.AppInstall) error {
	return r.db.Create(item).Error
}

func (r *AppInstallRepository) Update(item *model.AppInstall) error {
	return r.db.Save(item).Error
}

func (r *AppInstallRepository) Delete(id uint) error {
	return r.db.Delete(&model.AppInstall{}, id).Error
}

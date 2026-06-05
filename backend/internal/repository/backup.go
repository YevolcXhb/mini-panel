package repository

import (
	"gorm.io/gorm"
	"github.com/minipanel/minipanel/internal/model"
)

type BackupRepository struct {
	db *gorm.DB
}

func NewBackupRepository(db *gorm.DB) *BackupRepository {
	return &BackupRepository{db: db}
}

func (r *BackupRepository) CreateTask(item *model.BackupTask) error {
	return r.db.Create(item).Error
}

func (r *BackupRepository) ListTasks() ([]model.BackupTask, error) {
	var items []model.BackupTask
	err := r.db.Order("id DESC").Find(&items).Error
	return items, err
}

func (r *BackupRepository) GetTask(id uint) (*model.BackupTask, error) {
	var item model.BackupTask
	err := r.db.First(&item, id).Error
	return &item, err
}

func (r *BackupRepository) UpdateTask(item *model.BackupTask) error {
	return r.db.Save(item).Error
}

func (r *BackupRepository) DeleteTask(id uint) error {
	return r.db.Delete(&model.BackupTask{}, id).Error
}

func (r *BackupRepository) CreateRecord(item *model.BackupRecord) error {
	return r.db.Create(item).Error
}

func (r *BackupRepository) ListRecords(taskID uint) ([]model.BackupRecord, error) {
	var items []model.BackupRecord
	query := r.db.Order("id DESC")
	if taskID > 0 {
		query = query.Where("task_id = ?", taskID)
	}
	err := query.Find(&items).Error
	return items, err
}

func (r *BackupRepository) DeleteRecord(id uint) error {
	return r.db.Delete(&model.BackupRecord{}, id).Error
}

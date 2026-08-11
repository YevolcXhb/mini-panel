package service

import (
	"os"
	"path/filepath"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	"gorm.io/gorm"
)

type SettingService struct {
	repo *repository.SettingRepository
}

func NewSettingService() *SettingService {
	return &SettingService{
		repo: repository.NewSettingRepository(global.DB),
	}
}

func (s *SettingService) Get(key string) (string, error) {
	item, err := s.repo.Get(key)
	if err != nil {
		return "", err
	}
	return item.Value, nil
}

func (s *SettingService) Set(key, value string) error {
	return s.repo.Set(key, value)
}

func (s *SettingService) List() (map[string]string, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, item := range items {
		result[item.Key] = item.Value
	}
	return result, nil
}

func (s *SettingService) ClearData() error {
	models := []interface{}{
		&model.User{}, &model.App{}, &model.AppDetail{}, &model.AppInstall{},
		&model.AppSource{}, &model.Cronjob{}, &model.Setting{}, &model.LoginAttempt{}, &model.AuditLog{},
	}
	for _, m := range models {
		if err := global.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			return err
		}
	}
	logFile := filepath.Join(global.GetDataDir(), "logs", "panel.log")
	_ = os.Remove(logFile)
	if err := s.InitDefaults(); err != nil {
		return err
	}
	auth := NewAuthService()
	return auth.InitAdmin("admin", "admin123")
}

func (s *SettingService) InitDefaults() error {
	defaults := map[string]string{
		"theme":             "dark",
		"language":          "zh",
		"timezone":          "Asia/Shanghai",
		"container_mode":    "dockroot",
		"file_manager_root": "/",
		"SecurityEntrance":  "",
		"load_host_mode":    "chroot",
	}
	for k, v := range defaults {
		if _, err := s.repo.Get(k); err != nil {
			if err := s.repo.Set(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

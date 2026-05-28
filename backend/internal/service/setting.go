package service

import (
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/repository"
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

func (s *SettingService) InitDefaults() error {
	defaults := map[string]string{
		"theme":           "dark",
		"language":        "zh",
		"timezone":        "Asia/Shanghai",
		"container_mode":  "dockroot",
		"file_manager_root": "/",
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

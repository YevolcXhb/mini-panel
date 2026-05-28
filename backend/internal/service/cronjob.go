package service

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"

	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	"github.com/robfig/cron/v3"
)

type CronjobService struct {
	repo *repository.CronjobRepository
}

func NewCronjobService() *CronjobService {
	return &CronjobService{
		repo: repository.NewCronjobRepository(global.DB),
	}
}

func (s *CronjobService) List() ([]model.Cronjob, error) {
	return s.repo.List()
}

func (s *CronjobService) Create(req dto.CronjobCreateRequest) (*model.Cronjob, error) {
	item := &model.Cronjob{
		Name:    req.Name,
		Spec:    req.Spec,
		Command: req.Command,
		Script:  req.Script,
		Status:  "enabled",
	}
	if err := s.repo.Create(item); err != nil {
		return nil, err
	}
	if err := s.schedule(item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *CronjobService) Update(id uint, req dto.CronjobUpdateRequest) (*model.Cronjob, error) {
	item, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.Spec != "" {
		item.Spec = req.Spec
	}
	if req.Command != "" {
		item.Command = req.Command
	}
	if req.Script != "" {
		item.Script = req.Script
	}
	if req.Status != "" {
		item.Status = req.Status
	}

	if item.EntryID > 0 {
		global.Cron.Remove(cron.EntryID(item.EntryID))
		item.EntryID = 0
	}
	if item.Status == "enabled" {
		if err := s.schedule(item); err != nil {
			return nil, err
		}
	}
	return item, s.repo.Update(item)
}

func (s *CronjobService) Delete(id uint) error {
	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if item.EntryID > 0 {
		global.Cron.Remove(cron.EntryID(item.EntryID))
	}
	return s.repo.Delete(id)
}

func (s *CronjobService) Run(id uint) error {
	item, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	return s.execute(item)
}

func (s *CronjobService) schedule(item *model.Cronjob) error {
	entryID, err := global.Cron.AddFunc(item.Spec, func() {
		_ = s.execute(item)
	})
	if err != nil {
		return fmt.Errorf("invalid cron spec: %w", err)
	}
	item.EntryID = int(entryID)
	return s.repo.Update(item)
}

func (s *CronjobService) execute(item *model.Cronjob) error {
	item.LastRun = time.Now().Unix()
	var out bytes.Buffer
	var cmd *exec.Cmd
	if item.Script != "" {
		cmd = exec.Command("sh", "-c", item.Script)
	} else {
		cmd = exec.Command("sh", "-c", item.Command)
	}
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	item.LastLog = out.String()
	if err != nil {
		item.LastLog += fmt.Sprintf("\nError: %v", err)
	}
	return s.repo.Update(item)
}

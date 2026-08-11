package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

func (s *CronjobService) LoadAll() error {
	if global.Cron == nil || global.DB == nil {
		return nil
	}
	cronjobs, err := s.repo.List()
	if err != nil {
		return err
	}
	for i := range cronjobs {
		job := &cronjobs[i]
		if job.Status != "enabled" || job.EntryID > 0 {
			continue
		}
		if job.Spec == "" {
			continue
		}
		if err := s.schedule(job); err != nil {
			global.LOG.Warnf("Failed to load cronjob %d (%s): %v", job.ID, job.Name, err)
			continue
		}
		global.LOG.Infof("Loaded cronjob: %s (%s)", job.Name, job.Spec)
	}
	return nil
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
		global.LOG.Warnf("schedule cronjob %d failed: %v", item.ID, err)
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
	const execTimeout = 1 * time.Hour
	item.LastRun = time.Now().Unix()
	var out bytes.Buffer
	var cmd *exec.Cmd

	shell := "sh"
	shellArg := "-c"
	if runtime.GOOS == "windows" {
		shell = "cmd"
		shellArg = "/C"
	}

	if item.Script != "" {
		tmpFile, err := os.CreateTemp("", "cron-*.sh")
		if err != nil {
			item.LastLog = fmt.Sprintf("create temp script failed: %v", err)
			item.LastStatus = "failed"
			return s.repo.Update(item)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.WriteString(item.Script); err != nil {
			item.LastLog = fmt.Sprintf("write temp script failed: %v", err)
			item.LastStatus = "failed"
			return s.repo.Update(item)
		}
		tmpFile.Close()
		os.Chmod(tmpFile.Name(), 0755)
		ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, shell, tmpFile.Name())
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, shell, shellArg, item.Command)
	}
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	item.LastLog = out.String()
	if err != nil && strings.Contains(err.Error(), "context deadline exceeded") {
		item.LastLog += "\n[命令执行超时，已强制终止（最长 1 小时）]"
	}
	item.LastStatus = "success"
	if err != nil {
		item.LastLog += fmt.Sprintf("\nError: %v", err)
		item.LastStatus = "failed"
	}
	return s.repo.Update(item)
}

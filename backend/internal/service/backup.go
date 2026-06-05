package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
)

type BackupService struct {
	repo *repository.BackupRepository
}

func NewBackupService() *BackupService {
	return &BackupService{repo: repository.NewBackupRepository(global.DB)}
}

func (s *BackupService) ListTasks() ([]model.BackupTask, error) {
	return s.repo.ListTasks()
}

func (s *BackupService) CreateTask(item *model.BackupTask) error {
	if item.TargetDir == "" {
		item.TargetDir = "/data/backups"
	}
	if item.KeepCount <= 0 {
		item.KeepCount = 7
	}
	return s.repo.CreateTask(item)
}

func (s *BackupService) UpdateTask(item *model.BackupTask) error {
	return s.repo.UpdateTask(item)
}

func (s *BackupService) DeleteTask(id uint) error {
	return s.repo.DeleteTask(id)
}

func (s *BackupService) ListRecords(taskID uint) ([]model.BackupRecord, error) {
	return s.repo.ListRecords(taskID)
}

func (s *BackupService) DeleteRecord(id uint) error {
	return s.repo.DeleteRecord(id)
}

func (s *BackupService) RunBackup(taskID uint) (*model.BackupRecord, error) {
	task, err := s.repo.GetTask(taskID)
	if err != nil {
		return nil, err
	}

	record := &model.BackupRecord{
		TaskID:    taskID,
		Status:    "running",
		StartedAt: time.Now().Unix(),
	}
	if err := s.repo.CreateRecord(record); err != nil {
		return nil, err
	}

	task.LastStatus = "running"
	task.LastRunAt = time.Now().Unix()
	s.repo.UpdateTask(task)

	go s.doBackup(task, record)
	return record, nil
}

func (s *BackupService) doBackup(task *model.BackupTask, record *model.BackupRecord) {
	defer func() {
		record.FinishedAt = time.Now().Unix()
		s.repo.UpdateTask(task)
		s.repo.CreateRecord(record) // gorm will update existing record
	}()

	if err := os.MkdirAll(task.TargetDir, 0755); err != nil {
		record.Status = "failed"
		record.Message = fmt.Sprintf("create target dir failed: %v", err)
		task.LastStatus = "failed"
		task.LastMsg = record.Message
		return
	}

	timestamp := time.Now().Format("20060102_150405")
	var filePath string
	var cmd *exec.Cmd

	switch task.Type {
	case "website":
		websiteRepo := repository.NewWebsiteRepository(global.DB)
		website, err := websiteRepo.GetByID(task.SourceID)
		if err != nil {
			record.Status = "failed"
			record.Message = fmt.Sprintf("get website failed: %v", err)
			task.LastStatus = "failed"
			task.LastMsg = record.Message
			return
		}
		filePath = filepath.Join(task.TargetDir, fmt.Sprintf("website_%s_%s.tar.gz", website.Domain, timestamp))
		cmd = exec.Command("tar", "-czf", filePath, "-C", filepath.Dir(website.Root), filepath.Base(website.Root))

	case "database":
		dbRepo := repository.NewDatabaseRepository(global.DB)
		db, err := dbRepo.GetByID(task.SourceID)
		if err != nil {
			record.Status = "failed"
			record.Message = fmt.Sprintf("get database failed: %v", err)
			task.LastStatus = "failed"
			task.LastMsg = record.Message
			return
		}
		filePath = filepath.Join(task.TargetDir, fmt.Sprintf("db_%s_%s.sql", db.Name, timestamp))
		if db.Type == "mysql" {
			cmd = exec.Command("mysqldump", "-h", db.Host, "-P", strconv.Itoa(db.Port), "-u", db.Username, "-p"+db.Password, db.Database)
		} else if db.Type == "postgresql" {
			cmd = exec.Command("pg_dump", "-h", db.Host, "-p", strconv.Itoa(db.Port), "-U", db.Username, "-d", db.Database)
			cmd.Env = append(os.Environ(), "PGPASSWORD="+db.Password)
		} else {
			record.Status = "failed"
			record.Message = fmt.Sprintf("unsupported database type: %s", db.Type)
			task.LastStatus = "failed"
			task.LastMsg = record.Message
			return
		}
		// redirect output to file for database dump
		outFile, err := os.Create(filePath)
		if err != nil {
			record.Status = "failed"
			record.Message = fmt.Sprintf("create dump file failed: %v", err)
			task.LastStatus = "failed"
			task.LastMsg = record.Message
			return
		}
		defer outFile.Close()
		cmd.Stdout = outFile
		cmd.Stderr = outFile

	case "files":
		if task.SourcePath == "" {
			record.Status = "failed"
			record.Message = "source path is empty"
			task.LastStatus = "failed"
			task.LastMsg = record.Message
			return
		}
		base := filepath.Base(task.SourcePath)
		if base == "" || base == "/" {
			base = "backup"
		}
		filePath = filepath.Join(task.TargetDir, fmt.Sprintf("files_%s_%s.tar.gz", base, timestamp))
		cmd = exec.Command("tar", "-czf", filePath, "-C", filepath.Dir(task.SourcePath), filepath.Base(task.SourcePath))

	default:
		record.Status = "failed"
		record.Message = fmt.Sprintf("unknown backup type: %s", task.Type)
		task.LastStatus = "failed"
		task.LastMsg = record.Message
		return
	}

	if task.Type != "database" {
		out, err := cmd.CombinedOutput()
		if err != nil {
			record.Status = "failed"
			record.Message = fmt.Sprintf("backup failed: %v, output: %s", err, string(out))
			task.LastStatus = "failed"
			task.LastMsg = record.Message
			return
		}
	} else {
		if err := cmd.Run(); err != nil {
			record.Status = "failed"
			record.Message = fmt.Sprintf("database dump failed: %v", err)
			task.LastStatus = "failed"
			task.LastMsg = record.Message
			return
		}
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		record.Status = "failed"
		record.Message = fmt.Sprintf("stat backup file failed: %v", err)
		task.LastStatus = "failed"
		task.LastMsg = record.Message
		return
	}

	record.Status = "success"
	record.FilePath = filePath
	record.Size = stat.Size()
	record.Message = "Backup completed successfully"
	task.LastStatus = "success"
	task.LastMsg = record.Message

	// cleanup old backups
	s.cleanupOldBackups(task)
}

func (s *BackupService) cleanupOldBackups(task *model.BackupTask) {
	records, err := s.repo.ListRecords(task.ID)
	if err != nil {
		return
	}
	var successRecords []model.BackupRecord
	for _, r := range records {
		if r.Status == "success" && r.FilePath != "" {
			successRecords = append(successRecords, r)
		}
	}
	if len(successRecords) <= task.KeepCount {
		return
	}
	for i := task.KeepCount; i < len(successRecords); i++ {
		os.Remove(successRecords[i].FilePath)
		s.repo.DeleteRecord(successRecords[i].ID)
	}
}

func (s *BackupService) RestoreBackup(recordID uint) (string, error) {
	records, err := s.repo.ListRecords(0)
	if err != nil {
		return "", err
	}
	var target *model.BackupRecord
	for _, r := range records {
		if r.ID == recordID {
			target = &r
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("backup record not found")
	}
	if target.Status != "success" {
		return "", fmt.Errorf("cannot restore from failed backup")
	}

	task, err := s.repo.GetTask(target.TaskID)
	if err != nil {
		return "", err
	}

	var cmd *exec.Cmd
	switch task.Type {
	case "website", "files":
		if !strings.HasSuffix(target.FilePath, ".tar.gz") {
			return "", fmt.Errorf("unsupported backup format")
		}
		extractDir := task.SourcePath
		if task.Type == "website" {
			websiteRepo := repository.NewWebsiteRepository(global.DB)
			website, err := websiteRepo.GetByID(task.SourceID)
			if err != nil {
				return "", err
			}
			extractDir = website.Root
		}
		cmd = exec.Command("tar", "-xzf", target.FilePath, "-C", extractDir, "--strip-components=1")
	case "database":
		dbRepo := repository.NewDatabaseRepository(global.DB)
		db, err := dbRepo.GetByID(task.SourceID)
		if err != nil {
			return "", err
		}
		if db.Type == "mysql" {
			cmd = exec.Command("mysql", "-h", db.Host, "-P", strconv.Itoa(db.Port), "-u", db.Username, "-p"+db.Password, db.Database)
		} else if db.Type == "postgresql" {
			cmd = exec.Command("psql", "-h", db.Host, "-p", strconv.Itoa(db.Port), "-U", db.Username, "-d", db.Database)
			cmd.Env = append(os.Environ(), "PGPASSWORD="+db.Password)
		} else {
			return "", fmt.Errorf("unsupported database type: %s", db.Type)
		}
		input, err := os.Open(target.FilePath)
		if err != nil {
			return "", err
		}
		defer input.Close()
		cmd.Stdin = input
	default:
		return "", fmt.Errorf("unknown backup type: %s", task.Type)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("restore failed: %v, output: %s", err, string(out))
	}
	return "Restore completed successfully", nil
}

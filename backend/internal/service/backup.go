package service

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	syscmd "github.com/minipanel/minipanel/internal/utils/cmd"
	"github.com/robfig/cron/v3"
)

type BackupService struct {
	repo *repository.BackupRepository
}

func NewBackupService() *BackupService {
	return &BackupService{repo: repository.NewBackupRepository(global.DB)}
}

func (s *BackupService) LoadAll() error {
	if global.Cron == nil || global.DB == nil {
		return nil
	}
	tasks, err := s.repo.ListTasks()
	if err != nil {
		return err
	}
	for i := range tasks {
		task := &tasks[i]
		if !task.Enabled || task.Schedule == "" {
			continue
		}
		taskID := task.ID
		entryID, err := global.Cron.AddFunc(task.Schedule, func() {
			global.LOG.Infof("Running scheduled backup task: %s (ID: %d)", task.Name, taskID)
			_, err := s.RunBackup(taskID)
			if err != nil {
				global.LOG.Errorf("Scheduled backup task %d failed: %v", taskID, err)
			}
		})
		if err != nil {
			global.LOG.Warnf("Failed to load backup task %d (%s): %v", task.ID, task.Name, err)
			continue
		}
		task.Note = fmt.Sprintf("scheduled:entry_%d", entryID)
		_ = s.repo.UpdateTask(task)
		global.LOG.Infof("Loaded backup task: %s (%s)", task.Name, task.Schedule)
	}
	return nil
}

func (s *BackupService) Create(task *model.BackupTask) error {
	if task.TargetDir == "" {
		task.TargetDir = filepath.Join(global.GetDataDir(), "backups")
	}
	if task.KeepCount == 0 {
		task.KeepCount = 7
	}
	if err := os.MkdirAll(task.TargetDir, 0755); err != nil {
		return fmt.Errorf("create backup dir failed: %w", err)
	}
	if err := s.repo.CreateTask(task); err != nil {
		return err
	}
	if task.Enabled && task.Schedule != "" {
		taskID := task.ID
		entryID, err := global.Cron.AddFunc(task.Schedule, func() {
			global.LOG.Infof("Running scheduled backup task: %s (ID: %d)", task.Name, taskID)
			_, err := s.RunBackup(taskID)
			if err != nil {
				global.LOG.Errorf("Scheduled backup task %d failed: %v", taskID, err)
			}
		})
		if err != nil {
			global.LOG.Warnf("schedule backup task %d failed: %v", task.ID, err)
		} else {
			task.Note = fmt.Sprintf("scheduled:entry_%d", entryID)
			_ = s.repo.UpdateTask(task)
		}
	}
	return nil
}

func (s *BackupService) ListTasks() ([]model.BackupTask, error) {
	return s.repo.ListTasks()
}

func (s *BackupService) GetTaskByID(id uint) (*model.BackupTask, error) {
	return s.repo.GetTaskByID(id)
}

func (s *BackupService) UpdateTask(task *model.BackupTask) error {
	oldTask, err := s.repo.GetTaskByID(task.ID)
	if err != nil {
		return err
	}
	if strings.HasPrefix(oldTask.Note, "scheduled:entry_") {
		var entryID int
		fmt.Sscanf(oldTask.Note, "scheduled:entry_%d", &entryID)
		if entryID > 0 {
			global.Cron.Remove(cron.EntryID(entryID))
		}
	}
	if err := s.repo.UpdateTask(task); err != nil {
		return err
	}
	if task.Enabled && task.Schedule != "" {
		taskID := task.ID
		entryID, err := global.Cron.AddFunc(task.Schedule, func() {
			global.LOG.Infof("Running scheduled backup task: %s (ID: %d)", task.Name, taskID)
			_, err := s.RunBackup(taskID)
			if err != nil {
				global.LOG.Errorf("Scheduled backup task %d failed: %v", taskID, err)
			}
		})
		if err != nil {
			global.LOG.Warnf("schedule backup task %d failed: %v", task.ID, err)
		} else {
			task.Note = fmt.Sprintf("scheduled:entry_%d", entryID)
			_ = s.repo.UpdateTask(task)
		}
	}
	return nil
}

func (s *BackupService) DeleteTask(id uint) error {
	task, err := s.repo.GetTaskByID(id)
	if err != nil {
		return err
	}
	if strings.HasPrefix(task.Note, "scheduled:entry_") {
		var entryID int
		fmt.Sscanf(task.Note, "scheduled:entry_%d", &entryID)
		if entryID > 0 {
			global.Cron.Remove(cron.EntryID(entryID))
		}
	}
	records, err := s.repo.ListRecordsByTaskID(id)
	if err == nil {
		for _, rec := range records {
			_ = os.Remove(rec.FilePath)
			_ = s.repo.DeleteRecord(rec.ID)
		}
	}
	return s.repo.DeleteTask(task.ID)
}

func (s *BackupService) ListRecords(taskID uint) ([]model.BackupRecord, error) {
	if taskID > 0 {
		return s.repo.ListRecordsByTaskID(taskID)
	}
	return s.repo.ListAllRecords()
}

func (s *BackupService) RunBackup(taskID uint) (*model.BackupRecord, error) {
	task, err := s.repo.GetTaskByID(taskID)
	if err != nil {
		return nil, err
	}

	record := &model.BackupRecord{
		TaskID:    task.ID,
		Status:    "running",
		StartedAt: time.Now().Unix(),
	}
	if err := s.repo.CreateRecord(record); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(task.TargetDir, 0755); err != nil {
		s.markFailed(record, err)
		return record, err
	}

	var filePath string
	var size int64

	switch task.Type {
	case "files":
		filePath, size, err = s.backupFiles(task, record)
	case "website":
		filePath, size, err = s.backupWebsite(task, record)
	case "database":
		filePath, size, err = s.backupDatabase(task, record)
	default:
		err = fmt.Errorf("unknown backup type: %s", task.Type)
	}

	if err != nil {
		s.markFailed(record, err)
		s.cleanupOld(task)
		return record, err
	}

	record.FilePath = filePath
	record.Size = size
	record.Status = "success"
	record.FinishedAt = time.Now().Unix()
	record.Message = fmt.Sprintf("Backup completed successfully, size: %s", humanSize(size))
	_ = s.repo.UpdateRecord(record)

	task.LastRunAt = record.StartedAt
	task.LastStatus = "success"
	task.LastMsg = record.Message
	_ = s.repo.UpdateTask(task)

	s.cleanupOld(task)

	return record, nil
}

func (s *BackupService) markFailed(record *model.BackupRecord, err error) {
	record.Status = "failed"
	record.Message = err.Error()
	record.FinishedAt = time.Now().Unix()
	_ = s.repo.UpdateRecord(record)

	task, err := s.repo.GetTaskByID(record.TaskID)
	if err == nil {
		task.LastRunAt = record.StartedAt
		task.LastStatus = "failed"
		task.LastMsg = err.Error()
		_ = s.repo.UpdateTask(task)
	}
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (s *BackupService) backupFiles(task *model.BackupTask, record *model.BackupRecord) (string, int64, error) {
	source := task.SourcePath
	if source == "" {
		return "", 0, fmt.Errorf("source path is required")
	}
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return "", 0, fmt.Errorf("source path not found: %s", source)
	}

	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("%s_files_%s.zip", task.Name, timestamp)
	filePath := filepath.Join(task.TargetDir, fileName)

	f, err := os.Create(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate

		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})

	if err != nil {
		os.Remove(filePath)
		return "", 0, err
	}

	fi, _ := f.Stat()
	var size int64
	if fi != nil {
		size = fi.Size()
	}
	return filePath, size, nil
}

func (s *BackupService) backupWebsite(task *model.BackupTask, record *model.BackupRecord) (string, int64, error) {
	ws := NewWebsiteService()
	website, err := ws.GetByID(task.SourceID)
	if err != nil {
		return "", 0, fmt.Errorf("website not found: %w", err)
	}
	if website.Root == "" {
		website.Root = filepath.Join(global.GetDataDir(), "www", website.Domain)
	}
	task.SourcePath = website.Root
	return s.backupFiles(task, record)
}

func (s *BackupService) backupDatabase(task *model.BackupTask, record *model.BackupRecord) (string, int64, error) {
	dbSvc := NewDatabaseService()
	db, err := dbSvc.GetByID(task.SourceID)
	if err != nil {
		return "", 0, fmt.Errorf("database instance not found: %w", err)
	}

	if !syscmd.Which("mysqldump") {
		return "", 0, fmt.Errorf("mysqldump not found, please install mysql client")
	}

	timestamp := time.Now().Format("20060102_150405")
	dbName := db.Database
	if dbName == "" {
		dbName = "all"
	}
	var fileName string
	var filePath string
	var dumpFile string

	if runtime.GOOS == "windows" {
		fileName = fmt.Sprintf("%s_db_%s.sql", task.Name, timestamp)
		dumpFile = filepath.Join(task.TargetDir, fileName)
	} else {
		fileName = fmt.Sprintf("%s_db_%s.sql.gz", task.Name, timestamp)
		dumpFile = filepath.Join(task.TargetDir, fileName)
	}

	args := []string{
		fmt.Sprintf("-h%s", db.Host),
		fmt.Sprintf("-P%d", db.Port),
		fmt.Sprintf("-u%s", db.Username),
	}
	if db.Password != "" {
		args = append(args, fmt.Sprintf("-p%s", db.Password))
	}
	if db.Database != "" {
		args = append(args, db.Database)
	} else {
		args = append(args, "--all-databases")
	}
	args = append(args, "--single-transaction", "--quick", "--lock-tables=false")

	dumpCmd := exec.Command("mysqldump", args...)
	output, err := dumpCmd.CombinedOutput()
	if err != nil {
		return "", 0, fmt.Errorf("mysqldump failed: %s: %v", string(output), err)
	}

	if strings.HasSuffix(dumpFile, ".gz") && syscmd.Which("gzip") {
		gzCmd := exec.Command("gzip", "-c")
		gzCmd.Stdin = strings.NewReader(string(output))
		outFile, err := os.Create(dumpFile)
		if err != nil {
			return "", 0, err
		}
		defer outFile.Close()
		gzCmd.Stdout = outFile
		if err := gzCmd.Run(); err != nil {
			os.Remove(dumpFile)
			os.WriteFile(dumpFile, output, 0644)
		}
	} else {
		if err := os.WriteFile(dumpFile, output, 0644); err != nil {
			return "", 0, err
		}
	}

	filePath = dumpFile
	fi, err := os.Stat(filePath)
	if err != nil {
		return "", 0, err
	}
	return filePath, fi.Size(), nil
}

func (s *BackupService) cleanupOld(task *model.BackupTask) {
	if task.KeepCount <= 0 {
		return
	}
	records, err := s.repo.ListRecordsByTaskID(task.ID)
	if err != nil {
		return
	}
	if len(records) <= task.KeepCount {
		return
	}
	toDelete := records[task.KeepCount:]
	for _, rec := range toDelete {
		_ = os.Remove(rec.FilePath)
		_ = s.repo.DeleteRecord(rec.ID)
	}
}

func (s *BackupService) DeleteRecord(recordID uint) error {
	rec, err := s.repo.GetRecord(recordID)
	if err != nil {
		return err
	}
	_ = os.Remove(rec.FilePath)
	return s.repo.DeleteRecord(rec.ID)
}

func (s *BackupService) RestoreBackup(recordID uint) error {
	rec, err := s.repo.GetRecord(recordID)
	if err != nil {
		return err
	}
	task, err := s.repo.GetTaskByID(rec.TaskID)
	if err != nil {
		return err
	}

	if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", rec.FilePath)
	}

	switch task.Type {
	case "database":
		return s.restoreDatabase(task, rec)
	case "files", "website":
		return fmt.Errorf("file restore is not supported yet, please extract manually")
	default:
		return fmt.Errorf("unsupported backup type for restore")
	}
}

func (s *BackupService) restoreDatabase(task *model.BackupTask, rec *model.BackupRecord) error {
	if !syscmd.Which("mysql") {
		return fmt.Errorf("mysql client not found")
	}
	dbSvc := NewDatabaseService()
	db, err := dbSvc.GetByID(task.SourceID)
	if err != nil {
		return err
	}

	source := rec.FilePath
	var sqlContent []byte

	if strings.HasSuffix(rec.FilePath, ".gz") && syscmd.Which("gunzip") {
		gunzip := exec.Command("gunzip", "-c", rec.FilePath)
		sqlContent, err = gunzip.Output()
		if err != nil {
			return err
		}
	} else {
		sqlContent, err = os.ReadFile(source)
		if err != nil {
			return err
		}
	}

	args := []string{
		fmt.Sprintf("-h%s", db.Host),
		fmt.Sprintf("-P%d", db.Port),
		fmt.Sprintf("-u%s", db.Username),
	}
	if db.Password != "" {
		args = append(args, fmt.Sprintf("-p%s", db.Password))
	}
	if db.Database != "" {
		args = append(args, db.Database)
	}

	mysqlCmd := exec.Command("mysql", args...)
	mysqlCmd.Stdin = strings.NewReader(string(sqlContent))
	output, err := mysqlCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql restore failed: %s: %v", string(output), err)
	}
	return nil
}

package model

import "gorm.io/gorm"

type BackupTask struct {
	gorm.Model
	Name       string `json:"name" gorm:"not null"`
	Type       string `json:"type" gorm:"not null"` // website, database, files
	SourceID   uint   `json:"source_id"`            // website_id or database_id
	SourcePath string `json:"source_path"`          // custom file/dir path
	TargetDir  string `json:"target_dir" gorm:"default:/data/backups"`
	Schedule   string `json:"schedule"` // cron expression, empty = manual only
	KeepCount  int    `json:"keep_count" gorm:"default:7"`
	Enabled    bool   `json:"enabled" gorm:"default:true"`
	LastRunAt  int64  `json:"last_run_at"`
	LastStatus string `json:"last_status"` // pending, running, success, failed
	LastMsg    string `json:"last_msg"`
	Note       string `json:"note"`
}

type BackupRecord struct {
	gorm.Model
	TaskID     uint   `json:"task_id" gorm:"index;not null"`
	FilePath   string `json:"file_path"`
	Size       int64  `json:"size"`
	Status     string `json:"status" gorm:"default:running"` // running, success, failed
	Message    string `json:"message"`
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
}

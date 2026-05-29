package repository

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
)

func InitDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return db, nil
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.App{},
		&model.AppDetail{},
		&model.AppInstall{},
		&model.AppSource{},
		&model.Cronjob{},
		&model.Setting{},
		&model.LoginAttempt{},
		&model.AuditLog{},
	)
}

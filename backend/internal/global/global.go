package global

import (
	"io"
	"os"
	"path/filepath"

	"github.com/minipanel/minipanel/internal/config"
	"github.com/minipanel/minipanel/internal/repository"
	"github.com/minipanel/minipanel/internal/utils/dockroot"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var (
	CONF            *config.Config
	DB              *gorm.DB
	LOG             *logrus.Logger
	Cron            *cron.Cron
	IsAndroidChroot bool
	DockRootClient  *dockroot.Client
	Version         = "6.2.0"
	BuildTime       = "unknown"
	GitCommit       = "unknown"
)

func InitLogger(level string) error {
	LOG = logrus.New()
	LOG.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
	}
	LOG.SetLevel(lvl)

	logDir := filepath.Join(GetDataDir(), "logs")
	os.MkdirAll(logDir, 0755)
	logFile := filepath.Join(logDir, "panel.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		LOG.SetOutput(os.Stdout)
		return err
	}
	LOG.SetOutput(io.MultiWriter(os.Stdout, f))
	return nil
}

func InitDB(dbPath string) error {
	db, err := repository.InitDB(dbPath)
	if err != nil {
		return err
	}
	DB = db
	return nil
}

func MigrateDB() error {
	return repository.Migrate(DB)
}

func CloseDB() {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
}

func InitCron() error {
	Cron = cron.New()
	Cron.Start()
	return nil
}

func GetDataDir() string {
	if CONF != nil && CONF.DataDir != "" {
		return CONF.DataDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".minipanel")
}

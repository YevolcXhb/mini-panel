package global

import (
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
	CONF           *config.Config
	DB             *gorm.DB
	LOG            *logrus.Logger
	Cron           *cron.Cron
	IsAndroidChroot bool
	DockRootClient  *dockroot.Client
)

func InitLogger(level string) error {
	LOG = logrus.New()
	LOG.SetOutput(os.Stdout)
	LOG.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
	}
	LOG.SetLevel(lvl)
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

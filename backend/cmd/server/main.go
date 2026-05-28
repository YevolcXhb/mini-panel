package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/minipanel/minipanel/internal/config"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/router"
	"github.com/minipanel/minipanel/internal/service"
	"github.com/minipanel/minipanel/internal/utils/cmd"
	"github.com/minipanel/minipanel/internal/utils/dockroot"
	"github.com/sirupsen/logrus"
)

func main() {
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	global.CONF = cfg

	if err := global.InitLogger(cfg.LogLevel); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	if err := global.InitDB(cfg.DBPath); err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	if err := global.InitCron(); err != nil {
		return fmt.Errorf("init cron: %w", err)
	}

	global.IsAndroidChroot = detectAndroidChroot()
	global.LOG.Infof("Android chroot detected: %v", global.IsAndroidChroot)

	if cmd.Which("dockroot") {
		client, err := dockroot.NewClient()
		if err != nil {
			global.LOG.Warnf("dockroot not available: %v", err)
		} else {
			global.DockRootClient = client
			global.LOG.Info("dockroot client initialized")
		}
	}

	if err := global.MigrateDB(); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}

	// Init default settings and admin
	settingService := service.NewSettingService()
	_ = settingService.InitDefaults()
	authService := service.NewAuthService()
	_ = authService.InitAdmin("admin", "admin123")
	appService := service.NewAppService()
	_ = appService.InitDefaultApps()

	r := router.NewRouter()
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	global.LOG.Infof("mini-panel listening on http://%s", addr)
	return r.Run(addr)
}

func detectAndroidChroot() bool {
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(string(data), "Android") {
			return true
		}
	}
	return false
}

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/minipanel/minipanel/internal/config"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/router"
	"github.com/minipanel/minipanel/internal/service"
	"github.com/minipanel/minipanel/internal/utils/cmd"
	"github.com/minipanel/minipanel/internal/utils/dockroot"
	"github.com/sirupsen/logrus"
)

func init() {
	// Ignore SIGHUP to prevent accidental termination in chroot / nohup environments
	signal.Ignore(syscall.SIGHUP)
}

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	showVersion := flag.Bool("v", false, "show version")
	flag.Parse()
	if *showVersion {
		fmt.Printf("Mini Panel %s (commit: %s, built: %s)\n", version, gitCommit, buildTime)
		return
	}
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}

func run() error {
	logrus.Infof("Mini Panel %s (commit: %s, built: %s)", version, gitCommit, buildTime)

	exeDir := getExecutableDir()

	configPath := os.Getenv("MINIPANEL_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(exeDir, "config.yaml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.DBPath = absPath(exeDir, cfg.DBPath)
	cfg.DataDir = absPath(exeDir, cfg.DataDir)
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

	dockrootPath := findDockroot()
	if dockrootPath != "" {
		global.LOG.Infof("DockRoot found at: %s", dockrootPath)
		client, err := dockroot.NewClientWithPath(dockrootPath)
		if err != nil {
			global.LOG.Warnf("dockroot not available: %v", err)
		} else {
			global.DockRootClient = client
			global.LOG.Info("dockroot client initialized")
		}
	} else {
		global.LOG.Warn("DockRoot not found in PATH or local directory")
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
	_ = appService.InitDefaultSource()

	r := router.NewRouter()
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	global.LOG.Infof("mini-panel listening on http://%s", addr)
	return r.Run(addr)
}

func getExecutableDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(ex)
}

func absPath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func detectAndroidChroot() bool {
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/version"); err != nil {
		if strings.Contains(string(data), "Android") {
			return true
		}
	}
	return false
}

func findDockroot() string {
	localPath := filepath.Join(getExecutableDir(), "DockRoot")
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}
	if cmd.Which("dockroot") {
		return "dockroot"
	}
	if cmd.Which("DockRoot") {
		return "DockRoot"
	}
	return ""
}

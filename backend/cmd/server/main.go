package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/minipanel/minipanel/internal/agent/tools"
	"github.com/minipanel/minipanel/internal/config"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/router"
	"github.com/minipanel/minipanel/internal/service"
	"github.com/minipanel/minipanel/internal/utils/cmd"
	"github.com/minipanel/minipanel/internal/utils/dockroot"
	"github.com/sirupsen/logrus"
)

func init() {
	signal.Ignore(syscall.SIGHUP)
}

var (
	version   = "5.5.5"
	buildTime = "unknown"
	gitCommit = "unknown"
)

const (
	pidFile    = "minipanel.pid"
	defaultDir = "/opt/minipanel"
)

func main() {
	var (
		showVersion bool
		doStart     bool
		doStop      bool
		doRestart   bool
		doStatus    bool
		doUninstall bool
		doReset     bool
		doHelp      bool
		setsafe     string
	)

	flag.BoolVar(&showVersion, "v", false, "show version")
	flag.BoolVar(&doStart, "start", false, "start Mini Panel in background")
	flag.BoolVar(&doStop, "stop", false, "stop Mini Panel")
	flag.BoolVar(&doRestart, "restart", false, "restart Mini Panel")
	flag.BoolVar(&doStatus, "status", false, "check Mini Panel status")
	flag.BoolVar(&doUninstall, "uninstall", false, "uninstall Mini Panel")
	flag.BoolVar(&doReset, "reset", false, "reset admin password to admin123")
	flag.BoolVar(&doHelp, "help", false, "show help message")
	flag.StringVar(&setsafe, "setsafe", "", "set security entrance path (e.g. /1q2w3e)")
	flag.Parse()

	switch {
	case showVersion:
		fmt.Printf("Mini Panel %s (commit: %s, built: %s)\n", version, gitCommit, buildTime)
	case doStart:
		handleStart()
	case doStop:
		handleStop()
	case doRestart:
		handleRestart()
	case doStatus:
		handleStatus()
	case doUninstall:
		handleUninstall()
	case doReset:
		handleReset()
	case setsafe != "":
		handleSetsafe(setsafe)
	case doHelp:
		printHelp()
	default:
		if err := run(); err != nil {
			logrus.Fatal(err)
		}
	}
}

func exeDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(ex)
}

func pidPath() string {
	return filepath.Join(exeDir(), pidFile)
}

func readPID() int {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

func isRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func handleStart() {
	dir := exeDir()
	pid := readPID()
	if isRunning(pid) {
		fmt.Printf("Mini Panel is already running (PID: %d)\n", pid)
		return
	}
	os.Remove(pidPath())

	binary := filepath.Join(dir, "minipanel")
	c := exec.Command(binary)
	c.Dir = dir
	c.Stdout = nil
	c.Stderr = nil
	startProcess(c)
	if err := c.Start(); err != nil {
		fmt.Printf("Failed to start Mini Panel: %v\n", err)
		return
	}
	time.Sleep(200 * time.Millisecond)
	if isRunning(c.Process.Pid) {
		_ = os.WriteFile(pidPath(), []byte(strconv.Itoa(c.Process.Pid)), 0644)
		fmt.Printf("Mini Panel started (PID: %d)\n", c.Process.Pid)
	} else {
		fmt.Println("Mini Panel started")
	}
}

func handleStop() {
	pid := readPID()
	if !isRunning(pid) {
		fmt.Println("Mini Panel is not running")
		os.Remove(pidPath())
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("Failed to find process: %v\n", err)
		return
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("Failed to stop Mini Panel: %v\n", err)
		return
	}
	for i := 0; i < 20; i++ {
		if !isRunning(pid) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	os.Remove(pidPath())
	fmt.Println("Mini Panel stopped")
}

func handleRestart() {
	pid := readPID()
	if isRunning(pid) {
		process, err := os.FindProcess(pid)
		if err != nil {
			fmt.Printf("Failed to find process: %v\n", err)
			return
		}
		if err := process.Signal(syscall.SIGTERM); err != nil {
			fmt.Printf("Failed to stop Mini Panel: %v\n", err)
			return
		}
		for i := 0; i < 30; i++ {
			if !isRunning(pid) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		os.Remove(pidPath())
		fmt.Println("Mini Panel stopped")
		time.Sleep(500 * time.Millisecond)
	}
	handleStart()
}

func handleStatus() {
	pid := readPID()
	if isRunning(pid) {
		fmt.Printf("Mini Panel is running (PID: %d)\n", pid)
		ip := getLocalIP()
		fmt.Printf("Access: http://%s:8888\n", ip)
	} else {
		fmt.Println("Mini Panel is not running")
		os.Remove(pidPath())
	}
}

func handleUninstall() {
	dir := exeDir()
	if dir == "." {
		dir, _ = filepath.Abs(".")
	}

	pid := readPID()
	if isRunning(pid) {
		fmt.Println("Please stop Mini Panel first: minipanel --stop")
		return
	}

	fmt.Printf("This will remove Mini Panel from: %s\n", dir)
	fmt.Print("Are you sure? (y/N): ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled")
		return
	}

	os.Remove(filepath.Join(dir, pidFile))
	if err := os.RemoveAll(dir); err != nil {
		fmt.Printf("Failed to remove directory: %v\n", err)
		return
	}

	binPath := "/usr/local/bin/minipanel"
	os.Remove(binPath)
	drPath := "/usr/local/bin/dockroot"
	os.Remove(drPath)

	fmt.Printf("Mini Panel uninstalled from %s\n", dir)
}

func handleReset() {
	dir := exeDir()
	configPath := filepath.Join(dir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}
	cfg.DBPath = absPath(dir, cfg.DBPath)
	cfg.DataDir = absPath(dir, cfg.DataDir)
	global.CONF = cfg

	if err := global.InitDB(cfg.DBPath); err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		return
	}
	defer global.CloseDB()

	authService := service.NewAuthService()
	if err := authService.ResetPassword("admin", "admin123"); err != nil {
		fmt.Printf("Failed to reset password: %v\n", err)
		return
	}
	fmt.Println("Admin password reset to: admin123")
}

func handleSetsafe(path string) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	dir := exeDir()
	configPath := filepath.Join(dir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}
	cfg.DBPath = absPath(dir, cfg.DBPath)
	cfg.DataDir = absPath(dir, cfg.DataDir)
	global.CONF = cfg

	if err := global.InitDB(cfg.DBPath); err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		return
	}
	defer global.CloseDB()

	settingService := service.NewSettingService()
	if err := settingService.Set("SecurityEntrance", path); err != nil {
		fmt.Printf("Failed to set security entrance: %v\n", err)
		return
	}
	fmt.Printf("Security entrance set to: %s\n", path)
}

func printHelp() {
	fmt.Println("Mini Panel - Server Management Panel")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  minipanel              Start Mini Panel (foreground)")
	fmt.Println("  minipanel --start      Start Mini Panel in background")
	fmt.Println("  minipanel --stop       Stop Mini Panel")
	fmt.Println("  minipanel --restart    Restart Mini Panel")
	fmt.Println("  minipanel --status     Check Mini Panel status")
	fmt.Println("  minipanel --reset      Reset admin password to admin123")
	fmt.Println("  minipanel --setsafe    Set security entrance path")
	fmt.Println("  minipanel --uninstall  Uninstall Mini Panel")
	fmt.Println("  minipanel --help       Show this help message")
	fmt.Println("  minipanel -v           Show version")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  minipanel --start")
	fmt.Println("  minipanel --restart")
	fmt.Println("  minipanel --setsafe /1q2w3e")
	fmt.Println("")
	fmt.Printf("Install dir: %s\n", exeDir())
	fmt.Println("Default login: admin / admin123")
}

func getLocalIP() string {
	ip := os.Getenv("MINIPANEL_HOST")
	if ip != "" {
		return ip
	}
	return "localhost"
}

func run() error {
	global.Version = version
	global.BuildTime = buildTime
	global.GitCommit = gitCommit
	logrus.Infof("Mini Panel %s (commit: %s, built: %s)", global.Version, global.GitCommit, global.BuildTime)

	dir := exeDir()

	configPath := os.Getenv("MINIPANEL_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(dir, "config.yaml")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.DBPath = absPath(dir, cfg.DBPath)
	cfg.DataDir = absPath(dir, cfg.DataDir)
	global.CONF = cfg

	if err := global.InitLogger(cfg.LogLevel); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	if err := global.InitDB(cfg.DBPath); err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	// 初始化持久化 lazy-ref 存储（用于 agent 上下文压缩的大输出缓存）
	tools.InitPersistentLazyRefStore()
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

	cronSvc := service.NewCronjobService()
	if err := cronSvc.LoadAll(); err != nil {
		global.LOG.Warnf("load cronjobs failed: %v", err)
	}
	backupSvc := service.NewBackupService()
	if err := backupSvc.LoadAll(); err != nil {
		global.LOG.Warnf("load backup tasks failed: %v", err)
	}

	settingService := service.NewSettingService()
	_ = settingService.InitDefaults()
	authService := service.NewAuthService()
	_ = authService.InitAdmin("admin", "admin123")
	appService := service.NewAppService()
	_ = appService.InitDefaultApps()
	_ = appService.InitDefaultSource()

	service.StartMonitorCollector()

	r := router.NewRouter()
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Port)
	global.LOG.Infof("mini-panel listening on http://%s", addr)
	return r.Run(addr)
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
	if data, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(string(data), "Android") {
			return true
		}
	}
	// 共享内核的 Android 环境中，/proc/1/root 指向 Android init 的根目录
	if _, err := os.Stat("/proc/1/root/system"); err == nil {
		return true
	}
	return false
}

func findDockroot() string {
	localPath := filepath.Join(exeDir(), "DockRoot")
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

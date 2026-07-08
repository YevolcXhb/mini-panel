package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/minipanel/minipanel/internal/global"
)

// GitHub 仓库坐标（与 install.sh 保持一致）
const githubRepo = "YevolcXhb/mini-panel"

// UpdateInfo 检查更新结果
type UpdateInfo struct {
	CurrentVersion string `json:"current_version"` // 当前运行版本
	LatestVersion  string `json:"latest_version"`  // 最新版本
	HasUpdate      bool   `json:"has_update"`      // 是否有新版本
	ReleaseNotes   string `json:"release_notes"`   // 发布说明
	ReleaseURL     string `json:"release_url"`     // Release 页面 URL
	PublishedAt    string `json:"published_at"`    // 发布时间
	Source         string `json:"source"`          // 来源: "github" 或 "version.json"
}

// ApplyResult 应用更新结果
type ApplyResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	LogPath string `json:"log_path"` // 更新日志路径
	PID     int    `json:"pid"`      // 后台更新进程 PID
}

// UpdateService 更新服务
type UpdateService struct {
	httpClient *http.Client
}

// NewUpdateService 创建更新服务
func NewUpdateService() *UpdateService {
	return &UpdateService{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// CheckUpdate 检查最新版本。优先调用 GitHub API，失败时降级到 version.json。
func (s *UpdateService) CheckUpdate() (*UpdateInfo, error) {
	current := strings.TrimPrefix(global.Version, "v")
	info := &UpdateInfo{
		CurrentVersion: current,
	}

	// 方案1：GitHub Releases API
	if err := s.checkFromGitHub(info); err == nil && info.LatestVersion != "" {
		info.Source = "github"
		s.compareVersions(info)
		return info, nil
	}

	// 方案2：降级到 version.json
	if err := s.checkFromVersionJSON(info); err == nil && info.LatestVersion != "" {
		info.Source = "version.json"
		s.compareVersions(info)
		return info, nil
	}

	return nil, fmt.Errorf("无法获取最新版本信息（GitHub API 和 version.json 均失败）")
}

// githubReleaseResponse GitHub API 响应结构
type githubReleaseResponse struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// checkFromGitHub 从 GitHub Releases API 获取最新版本
func (s *UpdateService) checkFromGitHub(info *UpdateInfo) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MiniPanel/"+global.Version)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API 返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var release githubReleaseResponse
	if err := json.Unmarshal(body, &release); err != nil {
		return err
	}

	info.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	info.ReleaseNotes = release.Body
	info.ReleaseURL = release.HTMLURL
	info.PublishedAt = release.PublishedAt
	return nil
}

// versionJSONResponse version.json 结构
type versionJSONResponse struct {
	Version      string `json:"version"`
	URL          string `json:"url"`
	ReleaseNotes string `json:"release_notes"`
}

// checkFromVersionJSON 从 version.json 降级获取最新版本
func (s *UpdateService) checkFromVersionJSON(info *UpdateInfo) error {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/version.json", githubRepo)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("version.json 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("version.json 返回状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var vj versionJSONResponse
	if err := json.Unmarshal(body, &vj); err != nil {
		return err
	}

	info.LatestVersion = strings.TrimPrefix(vj.Version, "v")
	info.ReleaseNotes = vj.ReleaseNotes
	info.ReleaseURL = vj.URL
	return nil
}

// compareVersions 对比版本号，设置 HasUpdate
func (s *UpdateService) compareVersions(info *UpdateInfo) {
	latest := strings.TrimSpace(strings.TrimPrefix(info.LatestVersion, "v"))
	current := strings.TrimSpace(strings.TrimPrefix(info.CurrentVersion, "v"))
	if latest == "" {
		info.HasUpdate = false
		return
	}
	info.HasUpdate = compareSemver(latest, current) > 0
}

// compareSemver 比较两个 semver 版本号字符串。
// 返回 -1/0/1：a<b / a==b / a>b
func compareSemver(a, b string) int {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

// parseSemver 解析 "4.6.5" → [4,6,5]；非法部分按 0 处理
func parseSemver(s string) [3]int {
	var v [3]int
	parts := strings.Split(s, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		v[i] = n
	}
	return v
}

// updateState 更新过程中的状态机
var (
	updateMu    sync.Mutex
	updateState = struct {
		Running   bool
		StartTime time.Time
		LogPath   string
		PID       int
	}{}
)

// ApplyUpdate 下载并执行 install.sh 触发更新。
// 该方法立即返回，更新在后台进行；返回日志路径供前端轮询查看进度。
func (s *UpdateService) ApplyUpdate() (*ApplyResult, error) {
	updateMu.Lock()
	defer updateMu.Unlock()

	if updateState.Running {
		return nil, fmt.Errorf("已有更新任务在进行中（开始于 %s）", updateState.StartTime.Format("15:04:05"))
	}

	// 准备日志文件
	logDir := filepath.Join(global.GetDataDir(), "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	logPath := filepath.Join(logDir, "update.log")

	// 下载 install.sh
	installScriptURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/install.sh", githubRepo)
	global.LOG.Infof("[UpdateService] 下载 install.sh: %s", installScriptURL)

	scriptPath := filepath.Join(os.TempDir(), "minipanel-install.sh")
	if err := downloadFile(installScriptURL, scriptPath); err != nil {
		return nil, fmt.Errorf("下载 install.sh 失败: %w", err)
	}
	if err := os.Chmod(scriptPath, 0755); err != nil {
		return nil, fmt.Errorf("设置可执行权限失败: %w", err)
	}

	// 打开日志文件
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}

	// 构造命令（默认 NO_CHROOT=1 让 install.sh 不在 chroot 中运行）
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// 脱离父进程，避免更新过程中重启 minipanel 把更新进程也杀掉
	setSysProcAttr(cmd)
	// 继承环境变量，并设置更新模式标志
	cmd.Env = append(os.Environ(), "MINIPANEL_UPDATE=1")

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("启动更新进程失败: %w", err)
	}

	pid := cmd.Process.Pid
	updateState.Running = true
	updateState.StartTime = time.Now()
	updateState.LogPath = logPath
	updateState.PID = pid

	global.LOG.Infof("[UpdateService] 更新进程已启动 PID=%d, 日志=%s", pid, logPath)

	// 后台等待进程结束，清理状态
	go func() {
		_ = cmd.Wait()
		logFile.Close()
		updateMu.Lock()
		updateState.Running = false
		updateMu.Unlock()
		global.LOG.Infof("[UpdateService] 更新进程 PID=%d 已结束", pid)
	}()

	return &ApplyResult{
		Success: true,
		Message: "更新已开始，minipanel 服务将在编译完成后自动重启",
		LogPath: logPath,
		PID:     pid,
	}, nil
}

// GetUpdateStatus 获取当前更新任务状态
type UpdateStatus struct {
	Running   bool   `json:"running"`
	StartTime string `json:"start_time"`
	LogPath   string `json:"log_path"`
	PID       int    `json:"pid"`
}

func (s *UpdateService) GetStatus() *UpdateStatus {
	updateMu.Lock()
	defer updateMu.Unlock()
	return &UpdateStatus{
		Running:   updateState.Running,
		StartTime: updateState.StartTime.Format("2006-01-02 15:04:05"),
		LogPath:   updateState.LogPath,
		PID:       updateState.PID,
	}
}

// GetUpdateLog 读取更新日志（最后 N 行）
func (s *UpdateService) GetUpdateLog(tailLines int) (string, error) {
	if tailLines <= 0 {
		tailLines = 100
	}
	status := s.GetStatus()
	if status.LogPath == "" {
		return "", fmt.Errorf("暂无更新日志")
	}
	data, err := os.ReadFile(status.LogPath)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// downloadFile 下载文件到本地
func downloadFile(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

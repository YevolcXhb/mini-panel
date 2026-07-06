package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	syscmd "github.com/minipanel/minipanel/internal/utils/cmd"
)

type PhpService struct{}

func NewPhpService() *PhpService {
	return &PhpService{}
}

// GetInstalledVersions 获取已安装的 PHP 版本
func (s *PhpService) GetInstalledVersions() []model.PhpVersion {
	knownVersions := []string{"5.6", "7.0", "7.1", "7.2", "7.3", "7.4", "8.0", "8.1", "8.2", "8.3", "8.4"}
	var result []model.PhpVersion

	for _, v := range knownVersions {
		pv := model.PhpVersion{Version: v, Installed: false}
		binPath := fmt.Sprintf("/usr/bin/php%s", v)
		if _, err := os.Stat(binPath); err == nil {
			pv.Installed = true
			pv.BinPath = binPath
		} else {
			// 可能是默认 php 不带版本号
			binPath = "/usr/bin/php"
			if _, err := os.Stat(binPath); err == nil {
				// 检查版本号是否匹配
				out, err := exec.Command(binPath, "-v").CombinedOutput()
				if err == nil && strings.Contains(string(out), "PHP "+v) {
					pv.Installed = true
					pv.BinPath = binPath
				}
			}
		}

		if pv.Installed {
			// 检测 FPM socket
			socketPaths := []string{
				fmt.Sprintf("/run/php/php%s-fpm.sock", v),
				fmt.Sprintf("/var/run/php/php%s-fpm.sock", v),
				fmt.Sprintf("/run/php-fpm/php-fpm%s.sock", v),
				fmt.Sprintf("/tmp/php%s-fpm.sock", v),
			}
			for _, sp := range socketPaths {
				if _, err := os.Stat(sp); err == nil {
					pv.FpmSocket = sp
					break
				}
			}
			// 检测 FPM 是否运行
			svcName := fmt.Sprintf("php%s-fpm", v)
			pv.Running = s.isServiceRunning(svcName)

			// php.ini 路径
			iniPaths := []string{
				fmt.Sprintf("/etc/php/%s/fpm/php.ini", v),
				fmt.Sprintf("/etc/php%s/fpm/php.ini", v),
				fmt.Sprintf("/etc/php/%s/cli/php.ini", v),
				fmt.Sprintf("/etc/php.ini", v),
			}
			for _, ip := range iniPaths {
				if _, err := os.Stat(ip); err == nil {
					pv.PhpIniPath = ip
					break
				}
			}
		}
		result = append(result, pv)
	}
	return result
}

// isServiceRunning 检查 systemd 服务是否运行
func (s *PhpService) isServiceRunning(name string) bool {
	cmd := exec.Command("systemctl", "is-active", name)
	out, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

// InstallVersion 安装 PHP 版本
func (s *PhpService) InstallVersion(version string) error {
	global.LOG.Infof("[PHP] InstallVersion start: %s", version)
	if !syscmd.Which("apt") && !syscmd.Which("yum") && !syscmd.Which("dnf") {
		return fmt.Errorf("不支持的包管理器，仅支持 apt/yum/dnf")
	}

	// 检查是否已安装
	binPath := fmt.Sprintf("/usr/bin/php%s", version)
	if _, err := os.Stat(binPath); err == nil {
		return fmt.Errorf("PHP %s 已安装", version)
	}

	pkgs := []string{
		fmt.Sprintf("php%s-fpm", version),
		fmt.Sprintf("php%s-cli", version),
		fmt.Sprintf("php%s-common", version),
		fmt.Sprintf("php%s-mysql", version),
		fmt.Sprintf("php%s-curl", version),
		fmt.Sprintf("php%s-gd", version),
		fmt.Sprintf("php%s-mbstring", version),
		fmt.Sprintf("php%s-xml", version),
		fmt.Sprintf("php%s-zip", version),
		fmt.Sprintf("php%s-opcache", version),
		fmt.Sprintf("php%s-json", version),
	}

	var cmd *exec.Cmd
	if syscmd.Which("apt") {
		args := append([]string{"install", "-y"}, pkgs...)
		cmd = exec.Command("apt", args...)
	} else if syscmd.Which("dnf") {
		args := append([]string{"install", "-y"}, pkgs...)
		cmd = exec.Command("dnf", args...)
	} else {
		args := append([]string{"install", "-y"}, pkgs...)
		cmd = exec.Command("yum", args...)
	}

	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if len(output) > 500 {
		output = output[:500] + "..."
	}
	if err != nil {
		global.LOG.Errorf("[PHP] InstallVersion FAILED: %s: %v, output: %s", version, err, output)
		return fmt.Errorf("安装 PHP %s 失败: %v", version, err)
	}

	global.LOG.Infof("[PHP] InstallVersion OK: %s", version)
	return nil
}

// RemoveVersion 卸载 PHP 版本
func (s *PhpService) RemoveVersion(version string) error {
	global.LOG.Infof("[PHP] RemoveVersion start: %s", version)
	if !syscmd.Which("apt") && !syscmd.Which("yum") && !syscmd.Which("dnf") {
		return fmt.Errorf("不支持的包管理器")
	}

	var cmd *exec.Cmd
	if syscmd.Which("apt") {
		cmd = exec.Command("apt", "remove", "-y", fmt.Sprintf("php%s-*", version))
	} else if syscmd.Which("dnf") {
		cmd = exec.Command("dnf", "remove", "-y", fmt.Sprintf("php%s-*", version))
	} else {
		cmd = exec.Command("yum", "remove", "-y", fmt.Sprintf("php%s-*", version))
	}
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		global.LOG.Errorf("[PHP] RemoveVersion FAILED: %s: %v", version, err)
		return fmt.Errorf("卸载 PHP %s 失败: %s: %v", version, string(out), err)
	}
	global.LOG.Infof("[PHP] RemoveVersion OK: %s", version)
	return nil
}

// StartFpm 启动 PHP-FPM
func (s *PhpService) StartFpm(version string) error {
	svcName := fmt.Sprintf("php%s-fpm", version)
	return s.runSystemctl(svcName, "start")
}

// StopFpm 停止 PHP-FPM
func (s *PhpService) StopFpm(version string) error {
	svcName := fmt.Sprintf("php%s-fpm", version)
	return s.runSystemctl(svcName, "stop")
}

// RestartFpm 重启 PHP-FPM
func (s *PhpService) RestartFpm(version string) error {
	svcName := fmt.Sprintf("php%s-fpm", version)
	return s.runSystemctl(svcName, "restart")
}

func (s *PhpService) runSystemctl(name, action string) error {
	global.LOG.Infof("[PHP] %s %s", action, name)
	cmd := exec.Command("systemctl", action, name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		global.LOG.Errorf("[PHP] %s %s FAILED: %v, output: %s", action, name, err, string(out))
		return fmt.Errorf("%s %s 失败: %v", action, name, err)
	}
	global.LOG.Infof("[PHP] %s %s OK", action, name)
	return nil
}

// GetExtensions 获取 PHP 扩展列表
func (s *PhpService) GetExtensions(version string) ([]model.PhpExtension, error) {
	binPath := fmt.Sprintf("/usr/bin/php%s", version)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		binPath = "/usr/bin/php"
		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("PHP %s 未安装", version)
		}
	}

	out, err := exec.Command(binPath, "-m").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("获取扩展列表失败: %v", err)
	}

	lines := strings.Split(string(out), "\n")
	var exts []model.PhpExtension
	inExts := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "[PHP Modules]") {
			inExts = true
			continue
		}
		if strings.Contains(line, "[Zend Modules]") {
			inExts = false
			continue
		}
		if inExts {
			exts = append(exts, model.PhpExtension{Name: strings.ToLower(line), Installed: true})
		}
	}
	return exts, nil
}

// InstallExtension 安装 PHP 扩展
func (s *PhpService) InstallExtension(version, extName string) error {
	global.LOG.Infof("[PHP] InstallExtension: php%s ext=%s", version, extName)
	if !syscmd.Which("apt") && !syscmd.Which("yum") && !syscmd.Which("dnf") {
		return fmt.Errorf("不支持的包管理器")
	}

	pkgName := fmt.Sprintf("php%s-%s", version, extName)
	var cmd *exec.Cmd
	if syscmd.Which("apt") {
		cmd = exec.Command("apt", "install", "-y", pkgName)
	} else if syscmd.Which("dnf") {
		cmd = exec.Command("dnf", "install", "-y", pkgName)
	} else {
		cmd = exec.Command("yum", "install", "-y", pkgName)
	}

	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 尝试别名
		aliases := map[string]string{
			"redis":    "php-redis",
			"memcached": "php-memcached",
			"imagick":   "php-imagick",
			"mongodb":   "php-mongodb",
			"swoole":    "php-swoole",
			"xdebug":    "php-xdebug",
			"igbinary":  "php-igbinary",
			"msgpack":   "php-msgpack",
		}
		if alias, ok := aliases[extName]; ok && syscmd.Which("apt") {
			cmd = exec.Command("apt", "install", "-y", fmt.Sprintf("php%s-%s", version, alias))
			out, err = cmd.CombinedOutput()
		}
		if err != nil {
			global.LOG.Errorf("[PHP] InstallExtension FAILED: %s: %v", extName, err)
			return fmt.Errorf("安装扩展 %s 失败: %s: %v", extName, string(out), err)
		}
	}
	global.LOG.Infof("[PHP] InstallExtension OK: %s", extName)
	return nil
}

// RemoveExtension 卸载 PHP 扩展
func (s *PhpService) RemoveExtension(version, extName string) error {
	global.LOG.Infof("[PHP] RemoveExtension: php%s ext=%s", version, extName)
	pkgName := fmt.Sprintf("php%s-%s", version, extName)
	var cmd *exec.Cmd
	if syscmd.Which("apt") {
		cmd = exec.Command("apt", "remove", "-y", pkgName)
	} else if syscmd.Which("dnf") {
		cmd = exec.Command("dnf", "remove", "-y", pkgName)
	} else {
		cmd = exec.Command("yum", "remove", "-y", pkgName)
	}
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	out, err := cmd.CombinedOutput()
	if err != nil {
		global.LOG.Errorf("[PHP] RemoveExtension FAILED: %s: %v", extName, err)
		return fmt.Errorf("卸载扩展 %s 失败: %s: %v", extName, string(out), err)
	}
	global.LOG.Infof("[PHP] RemoveExtension OK: %s", extName)
	return nil
}

// GetPhpIni 获取 php.ini 配置
func (s *PhpService) GetPhpIni(version string) ([]model.PhpConfigItem, error) {
	iniPath := s.findPhpIni(version)
	if iniPath == "" {
		return nil, fmt.Errorf("找不到 PHP %s 的 php.ini", version)
	}
	data, err := os.ReadFile(iniPath)
	if err != nil {
		return nil, err
	}

	// 常见配置项
	keys := []string{
		"upload_max_filesize", "post_max_size", "max_execution_time",
		"max_input_time", "memory_limit", "max_input_vars",
		"date.timezone", "display_errors", "display_startup_errors",
		"error_reporting", "log_errors", "error_log",
		"default_charset", "expose_php", "short_open_tag",
		"opcache.enable", "opcache.memory_consumption",
		"session.save_handler", "session.save_path",
	}
	content := string(data)
	var items []model.PhpConfigItem
	for _, key := range keys {
		// 简单解析 ini 值
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if strings.TrimSpace(parts[0]) == key {
				items = append(items, model.PhpConfigItem{
					Key:   key,
					Value: strings.TrimSpace(parts[1]),
				})
				break
			}
		}
	}
	return items, nil
}

// UpdatePhpIni 修改 php.ini 配置项
func (s *PhpService) UpdatePhpIni(version string, items []model.PhpConfigItem) error {
	iniPath := s.findPhpIni(version)
	if iniPath == "" {
		return fmt.Errorf("找不到 PHP %s 的 php.ini", version)
	}
	data, err := os.ReadFile(iniPath)
	if err != nil {
		return err
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	for _, item := range items {
		found := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
				continue
			}
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if strings.TrimSpace(parts[0]) == item.Key {
				lines[i] = fmt.Sprintf("%s = %s", item.Key, item.Value)
				found = true
				break
			}
		}
		if !found {
			lines = append(lines, fmt.Sprintf("%s = %s", item.Key, item.Value))
		}
	}

	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(iniPath, []byte(newContent), 0644); err != nil {
		return err
	}
	global.LOG.Infof("[PHP] UpdatePhpIni OK: %s (%d items)", iniPath, len(items))
	return nil
}

func (s *PhpService) findPhpIni(version string) string {
	paths := []string{
		fmt.Sprintf("/etc/php/%s/fpm/php.ini", version),
		fmt.Sprintf("/etc/php%s/fpm/php.ini", version),
		fmt.Sprintf("/etc/php/%s/cli/php.ini", version),
		fmt.Sprintf("/etc/php.ini"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// 通过 php --ini 获取
	binPath := fmt.Sprintf("/usr/bin/php%s", version)
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		binPath = "/usr/bin/php"
	}
	out, err := exec.Command(binPath, "--ini").CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Loaded Configuration File:") {
				p := strings.TrimSpace(strings.TrimPrefix(line, "Loaded Configuration File:"))
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	return ""
}

// GetFpmSocket 获取 PHP-FPM socket 路径
func (s *PhpService) GetFpmSocket(version string) string {
	paths := []string{
		fmt.Sprintf("/run/php/php%s-fpm.sock", version),
		fmt.Sprintf("/var/run/php/php%s-fpm.sock", version),
		fmt.Sprintf("/run/php-fpm/php-fpm%s.sock", version),
		fmt.Sprintf("/tmp/php%s-fpm.sock", version),
		fmt.Sprintf("unix:/run/php/php%s-fpm.sock", version),
	}
	for _, p := range paths {
		if _, err := os.Stat(strings.TrimPrefix(p, "unix:")); err == nil {
			return p
		}
	}
	// 默认返回 unix socket 路径
	return fmt.Sprintf("unix:/run/php/php%s-fpm.sock", version)
}

// SetFpmPool 为网站设置 PHP-FPM 池配置
func (s *PhpService) SetFpmPool(version, domain, root string, port int) error {
	// 查找 FPM pool 配置目录
	poolDirs := []string{
		fmt.Sprintf("/etc/php/%s/fpm/pool.d", version),
		fmt.Sprintf("/etc/php%s/fpm/pool.d", version),
		fmt.Sprintf("/etc/php-fpm.d", version),
	}
	var poolDir string
	for _, d := range poolDirs {
		if stat, err := os.Stat(d); err == nil && stat.IsDir() {
			poolDir = d
			break
		}
	}
	if poolDir == "" {
		global.LOG.Warnf("[PHP] SetFpmPool: no pool.d dir found for php%s", version)
		return nil // 非致命错误
	}

	poolName := strings.ReplaceAll(domain, ".", "_")
	poolPath := filepath.Join(poolDir, poolName+".conf")

	pool := fmt.Sprintf(`; Managed by MiniPanel
[%s]
user = www-data
group = www-data
listen = /run/php/php%s-fpm-%s.sock
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = dynamic
pm.max_children = 50
pm.start_servers = 5
pm.min_spare_servers = 5
pm.max_spare_servers = 35
pm.max_requests = 500
chdir = /
security.limit_extensions = .php .php5 .php7 .phtml
php_admin_value[open_basedir] = %s:/tmp:/usr/share/php
`, poolName, version, poolName, root)

	if err := os.WriteFile(poolPath, []byte(pool), 0644); err != nil {
		global.LOG.Errorf("[PHP] SetFpmPool write failed: %v", err)
		return err
	}
	global.LOG.Infof("[PHP] SetFpmPool OK: domain=%s pool=%s", domain, poolPath)
	return nil
}
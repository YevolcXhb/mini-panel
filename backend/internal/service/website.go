package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	syscmd "github.com/minipanel/minipanel/internal/utils/cmd"
)

type WebsiteService struct {
	repo *repository.WebsiteRepository
}

func NewWebsiteService() *WebsiteService {
	return &WebsiteService{repo: repository.NewWebsiteRepository(global.DB)}
}

func (s *WebsiteService) Create(w *model.Website) error {
	w.Managed = true
	if err := s.repo.Create(w); err != nil {
		return err
	}
	return s.applyConfig(w)
}

func (s *WebsiteService) Update(w *model.Website) error {
	old, err := s.repo.GetByID(w.ID)
	if err != nil {
		return err
	}
	if !old.Managed {
		return fmt.Errorf("外部创建的网站不可通过面板修改配置")
	}
	if old.Domain != w.Domain {
		_ = s.removeConfig(old)
	}
	if err := s.repo.Update(w); err != nil {
		return err
	}
	return s.applyConfig(w)
}

func (s *WebsiteService) Delete(id uint) error {
	w, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if w.Managed {
		_ = s.removeConfig(w)
	}
	return s.repo.Delete(id)
}

func (s *WebsiteService) GetByID(id uint) (*model.Website, error) {
	return s.repo.GetByID(id)
}

// List 获取所有网站，包括数据库中面板创建的和nginx中外部创建的
func (s *WebsiteService) List() ([]model.Website, error) {
	// 1. 获取数据库中的网站
	dbSites, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	// 2. 扫描nginx配置中的网站
	scannedSites := s.scanNginxWebsites()

	// 3. 合并：数据库中已有的不重复添加，扫描到的新网站添加为非托管
	existing := make(map[string]bool)
	for _, site := range dbSites {
		key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
		existing[key] = true
	}

	for _, site := range scannedSites {
		key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
		if !existing[key] {
			dbSites = append(dbSites, site)
			existing[key] = true
		}
	}

	return dbSites, nil
}

func (s *WebsiteService) ToggleEnable(id uint, enabled bool) error {
	w, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if !w.Managed {
		return fmt.Errorf("外部创建的网站不可通过面板启停，请直接修改nginx配置")
	}
	w.Enabled = enabled
	if err := s.repo.Update(w); err != nil {
		return err
	}
	return s.applyConfig(w)
}

// GetNginxStatus 获取Nginx服务状态
func (s *WebsiteService) GetNginxStatus() (*model.NginxStatus, error) {
	status := &model.NginxStatus{
		Installed: false,
		Running:   false,
		Version:   "",
		ConfigDir: s.GetNginxConfigDir(),
		Pid:       0,
	}

	// 检查nginx是否安装
	if !syscmd.Which("nginx") {
		return status, nil
	}
	status.Installed = true

	// 获取版本
	output, err := exec.Command("nginx", "-v").CombinedOutput()
	if err == nil {
		verStr := string(output)
		re := regexp.MustCompile(`nginx/(\d+\.\d+\.\d+)`)
		if matches := re.FindStringSubmatch(verStr); len(matches) >= 2 {
			status.Version = matches[1]
		}
	}

	// 检查是否运行
	pidFile := s.findNginxPidFile()
	if pidFile != "" {
		pidData, err := os.ReadFile(pidFile)
		if err == nil {
			pidStr := strings.TrimSpace(string(pidData))
			pid, err := strconv.Atoi(pidStr)
			if err == nil {
				// 检查进程是否存在
				process, err := os.FindProcess(pid)
				if err == nil {
					// 发送信号0检查进程是否存在
					if err := process.Signal(os.Signal(nil)); err == nil {
						status.Running = true
						status.Pid = pid
					}
				}
			}
		}
	}

	// 如果pid文件方式没找到，尝试ps命令
	if !status.Running {
		output, err := exec.Command("ps", "aux").CombinedOutput()
		if err == nil {
			if strings.Contains(string(output), "nginx: master") {
				status.Running = true
				// 尝试提取pid
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					if strings.Contains(line, "nginx: master") && !strings.Contains(line, "grep") {
						fields := strings.Fields(line)
						for i, f := range fields {
							if i == 1 {
								if pid, err := strconv.Atoi(f); err == nil {
									status.Pid = pid
								}
								break
							}
						}
						break
					}
				}
			}
		}
	}

	return status, nil
}

func (s *WebsiteService) findNginxPidFile() string {
	candidates := []string{
		"/var/run/nginx.pid",
		"/run/nginx.pid",
		"/usr/local/nginx/logs/nginx.pid",
		"/var/log/nginx/nginx.pid",
	}
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	// 尝试从nginx配置中查找
	output, err := exec.Command("nginx", "-T").CombinedOutput()
	if err == nil {
		re := regexp.MustCompile(`pid\s+([^;]+);`)
		if matches := re.FindStringSubmatch(string(output)); len(matches) >= 2 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

// StartNginx 启动Nginx
func (s *WebsiteService) StartNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx 未安装")
	}
	// 先测试配置
	output, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx配置测试失败: %s", string(output))
	}
	// 尝试systemctl启动
	if err := exec.Command("systemctl", "start", "nginx").Run(); err == nil {
		return nil
	}
	// 直接启动
	cmd := exec.Command("nginx")
	return cmd.Run()
}

// StopNginx 停止Nginx
func (s *WebsiteService) StopNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx 未安装")
	}
	// 尝试systemctl停止
	if err := exec.Command("systemctl", "stop", "nginx").Run(); err == nil {
		return nil
	}
	// 发送stop信号
	output, err := exec.Command("nginx", "-s", "stop").CombinedOutput()
	if err != nil {
		// 如果nginx -s stop失败，尝试kill进程
		pidFile := s.findNginxPidFile()
		if pidFile != "" {
			pidData, err := os.ReadFile(pidFile)
			if err == nil {
				pidStr := strings.TrimSpace(string(pidData))
				if pid, err := strconv.Atoi(pidStr); err == nil {
					process, err := os.FindProcess(pid)
					if err == nil {
						return process.Kill()
					}
				}
			}
		}
		return fmt.Errorf("停止nginx失败: %s", string(output))
	}
	return nil
}

// RestartNginx 重启Nginx
func (s *WebsiteService) RestartNginx() error {
	// 先停止
	_ = s.StopNginx()
	// 等待一下
	// time.Sleep(500 * time.Millisecond)
	// 启动
	return s.StartNginx()
}

func (s *WebsiteService) ReloadNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx is not installed")
	}
	output, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx config test failed: %s", string(output))
	}
	// 尝试reload
	reloadCmd := exec.Command("nginx", "-s", "reload")
	if _, err := reloadCmd.CombinedOutput(); err != nil {
		_ = exec.Command("systemctl", "reload", "nginx").Run()
		_ = exec.Command("systemctl", "restart", "nginx").Run()
	}
	return nil
}

func (s *WebsiteService) GetNginxConfigDir() string {
	candidates := []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/usr/local/nginx/conf/conf.d",
		"/usr/local/nginx/conf/vhost",
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	// 尝试从nginx -T输出获取配置目录
	output, err := exec.Command("nginx", "-T").CombinedOutput()
	if err == nil {
		content := string(output)
		// 查找include配置
		re := regexp.MustCompile(`include\s+([^;]+/conf\.d|\*/conf\.d|\*/sites-enabled);`)
		if matches := re.FindStringSubmatch(content); len(matches) >= 2 {
			dir := strings.TrimSpace(matches[1])
			dir = strings.ReplaceAll(dir, "*", "/etc/nginx")
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}
	panelDir := filepath.Join(global.GetDataDir(), "nginx")
	_ = os.MkdirAll(panelDir, 0755)
	return panelDir
}

// scanNginxWebsites 扫描所有nginx配置文件，提取网站信息
func (s *WebsiteService) scanNginxWebsites() []model.Website {
	var sites []model.Website
	seen := make(map[string]bool)

	configDirs := []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/usr/local/nginx/conf/conf.d",
		"/usr/local/nginx/conf/vhost",
	}
	configDirs = append(configDirs, s.GetNginxConfigDir())

	for _, dir := range configDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".conf") {
				return nil
			}
			parsedSites := s.parseNginxConfig(path)
			for _, site := range parsedSites {
				key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
				if !seen[key] {
					sites = append(sites, site)
					seen[key] = true
				}
			}
			return nil
		})
	}

	// 也扫描主配置文件中的server块
	mainConf := "/etc/nginx/nginx.conf"
	if _, err := os.Stat(mainConf); err == nil {
		parsedSites := s.parseNginxConfig(mainConf)
		for _, site := range parsedSites {
			key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
			if !seen[key] {
				sites = append(sites, site)
				seen[key] = true
			}
		}
	}

	return sites
}

// parseNginxConfig 解析nginx配置文件，提取所有server块信息
func (s *WebsiteService) parseNginxConfig(configPath string) []model.Website {
	var sites []model.Website
	file, err := os.Open(configPath)
	if err != nil {
		return sites
	}
	defer file.Close()

	configFileName := filepath.Base(configPath)
	configFileName = strings.TrimSuffix(configFileName, ".conf")

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var (
		inServerBlock bool
		braceDepth    int
		serverName    string
		listenPorts   []int
		rootDir       string
		proxyPass     string
		hasSSL        bool
	)

	commentRe := regexp.MustCompile(`#.*$`)
	serverNameRe := regexp.MustCompile(`server_name\s+([^;]+);`)
	listenRe := regexp.MustCompile(`listen\s+(\d+)(.*);`)
	rootRe := regexp.MustCompile(`root\s+([^;]+);`)
	proxyPassRe := regexp.MustCompile(`proxy_pass\s+([^;]+);`)

	for scanner.Scan() {
		line := scanner.Text()
		line = commentRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if !inServerBlock {
			if strings.Contains(line, "server") && strings.Contains(line, "{") {
				inServerBlock = true
				braceDepth = strings.Count(line, "{") - strings.Count(line, "}")
				serverName = ""
				listenPorts = nil
				rootDir = ""
				proxyPass = ""
				hasSSL = false
			}
			continue
		}

		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

		if matches := serverNameRe.FindStringSubmatch(line); len(matches) >= 2 {
			names := strings.Fields(matches[1])
			for _, name := range names {
				name = strings.TrimSpace(name)
				if name != "" && name != "_" && !strings.HasPrefix(name, "*.") && !strings.HasPrefix(name, "~") {
					serverName = name
					break
				}
			}
			if serverName == "" && len(names) > 0 {
				first := strings.TrimSpace(names[0])
				if first != "_" {
					serverName = first
				}
			}
		}

		if matches := listenRe.FindStringSubmatch(line); len(matches) >= 2 {
			portStr := strings.TrimSpace(matches[1])
			port, err := strconv.Atoi(portStr)
			if err == nil {
				listenPorts = append(listenPorts, port)
			}
			if strings.Contains(matches[2], "ssl") {
				hasSSL = true
			}
		}

		if matches := rootRe.FindStringSubmatch(line); len(matches) >= 2 {
			rootDir = strings.TrimSpace(matches[1])
		}

		if matches := proxyPassRe.FindStringSubmatch(line); len(matches) >= 2 {
			proxyPass = strings.TrimSpace(matches[1])
		}

		if braceDepth <= 0 {
			inServerBlock = false

			if len(listenPorts) > 0 {
				if serverName == "" || serverName == "_" {
					serverName = configFileName
					if serverName == "nginx" {
						serverName = "default"
					}
				}

				port := listenPorts[0]
				if hasSSL && len(listenPorts) > 1 {
					for _, p := range listenPorts {
						if p != 443 {
							port = p
							break
						}
					}
				}

				siteType := "static"
				if proxyPass != "" {
					siteType = "proxy"
				}

				name := serverName
				if serverName == "default" {
					name = "默认站点"
				}

				sites = append(sites, model.Website{
					Name:        name,
					Domain:      serverName,
					Port:        port,
					Root:        rootDir,
					Type:        siteType,
					ProxyTarget: proxyPass,
					SSL:         hasSSL,
					Enabled:     true,
					Managed:     false,
					ConfigFile:  configPath,
					Remark:      "检测到外部创建的网站",
				})
			}
		}
	}

	return sites
}

func testWritable(dir string) bool {
	testFile := filepath.Join(dir, ".minipanel_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return false
	}
	os.Remove(testFile)
	return true
}

func (s *WebsiteService) nginxConfPath(w *model.Website) string {
	confDir := s.GetNginxConfigDir()
	return filepath.Join(confDir, fmt.Sprintf("%s.conf", w.Domain))
}

func (s *WebsiteService) removeConfig(w *model.Website) error {
	path := s.nginxConfPath(w)
	_ = os.Remove(path)
	return nil
}

func (s *WebsiteService) applyConfig(w *model.Website) error {
	if !syscmd.Which("nginx") {
		global.LOG.Warn("nginx not installed, config saved but not applied")
		return nil
	}

	if !w.Enabled {
		if err := s.removeConfig(w); err != nil {
			return err
		}
		return s.ReloadNginx()
	}

	confDir := s.GetNginxConfigDir()
	_ = os.MkdirAll(confDir, 0755)

	var sb strings.Builder
	port := w.Port
	if port == 0 {
		port = 80
	}

	sb.WriteString("# Managed by MiniPanel\n")
	sb.WriteString("server {\n")
	if w.SSL {
		sb.WriteString(fmt.Sprintf("    listen %d;\n", port))
		sb.WriteString(fmt.Sprintf("    listen [::]:%d;\n", port))
		sb.WriteString(fmt.Sprintf("    server_name %s;\n", w.Domain))
		sb.WriteString("    return 301 https://$server_name$request_uri;\n")
		sb.WriteString("}\n\n")

		sb.WriteString("server {\n")
		sb.WriteString(fmt.Sprintf("    listen %d ssl http2;\n", port+443))
		sb.WriteString(fmt.Sprintf("    listen [::]:%d ssl http2;\n", port+443))
		sb.WriteString(fmt.Sprintf("    server_name %s;\n", w.Domain))
		if w.SSLCert != "" {
			sb.WriteString(fmt.Sprintf("    ssl_certificate %s;\n", w.SSLCert))
		}
		if w.SSLKey != "" {
			sb.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n", w.SSLKey))
		}
		sb.WriteString("    ssl_protocols TLSv1.2 TLSv1.3;\n")
		sb.WriteString("    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;\n")
		sb.WriteString("    ssl_prefer_server_ciphers on;\n")
	} else {
		sb.WriteString(fmt.Sprintf("    listen %d;\n", port))
		sb.WriteString(fmt.Sprintf("    listen [::]:%d;\n", port))
		sb.WriteString(fmt.Sprintf("    server_name %s;\n", w.Domain))
	}

	if w.Type == "proxy" && w.ProxyTarget != "" {
		sb.WriteString("\n    location / {\n")
		sb.WriteString(fmt.Sprintf("        proxy_pass %s;\n", w.ProxyTarget))
		sb.WriteString("        proxy_set_header Host $host;\n")
		sb.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
		sb.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
		sb.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
		sb.WriteString("    }\n")
	} else {
		root := w.Root
		if root == "" {
			root = filepath.Join(global.GetDataDir(), "www", w.Domain)
		}
		_ = os.MkdirAll(root, 0755)
		indexFile := filepath.Join(root, "index.html")
		if _, err := os.Stat(indexFile); os.IsNotExist(err) {
			defaultHtml := fmt.Sprintf(`<html>
<head><title>Welcome to %s</title></head>
<body>
<h1>Website %s is running!</h1>
<p>Managed by MiniPanel.</p>
</body>
</html>`, w.Domain, w.Domain)
			os.WriteFile(indexFile, []byte(defaultHtml), 0644)
		}
		sb.WriteString(fmt.Sprintf("\n    root %s;\n", root))
		sb.WriteString("    index index.html index.htm index.php;\n")
		sb.WriteString("\n    location / {\n")
		sb.WriteString("        try_files $uri $uri/ =404;\n")
		sb.WriteString("    }\n")
	}

	sb.WriteString("}\n")

	path := s.nginxConfPath(w)
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write nginx config failed: %w", err)
	}

	if err := s.ReloadNginx(); err != nil {
		global.LOG.Warnf("nginx reload failed: %v", err)
	}
	return nil
}

package service

import (
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

	// 优先通过 nginx -T 拿到合并后的完整配置（包含所有 include 的文件）
	if mergedSites := s.parseNginxMergedConfig(); len(mergedSites) > 0 {
		for _, site := range mergedSites {
			key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
			if !seen[key] {
				sites = append(sites, site)
				seen[key] = true
			}
		}
		return sites
	}

	// 退路：直接遍历常见配置目录
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

// parseNginxMergedConfig 通过 nginx -T 获取合并后的完整配置并解析
func (s *WebsiteService) parseNginxMergedConfig() []model.Website {
	if !syscmd.Which("nginx") {
		return nil
	}
	output, err := exec.Command("nginx", "-T").CombinedOutput()
	if err != nil {
		return nil
	}
	content := string(output)
	if len(strings.TrimSpace(content)) == 0 {
		return nil
	}

	// 优先用 # configuration file 注释拆分（nginx -T 自带标记）
	parts := expandNginxMergedConfig(content)
	if len(parts) > 1 {
		var sites []model.Website
		seen := make(map[string]bool)
		for file, body := range parts {
			if file == "__main__" {
				continue
			}
			fileSites := s.parseNginxConfigContent(body, file)
			for _, site := range fileSites {
				key := fmt.Sprintf("%s:%d", site.Domain, site.Port)
				if !seen[key] {
					sites = append(sites, site)
					seen[key] = true
				}
			}
		}
		if len(sites) > 0 {
			return sites
		}
	}
	// 回退：当作单一文件解析
	return s.parseNginxConfigContent(content, "")
}

// parseNginxConfig 解析nginx配置文件，提取所有server块信息
func (s *WebsiteService) parseNginxConfig(configPath string) []model.Website {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	fileName := filepath.Base(configPath)
	fileName = strings.TrimSuffix(fileName, ".conf")
	return s.parseNginxConfigContent(string(data), fileName)
}

// parseNginxConfigContent 解析nginx配置内容，使用栈跟踪 server 块和 include 上下文
func (s *WebsiteService) parseNginxConfigContent(content string, defaultName string) []model.Website {
	var sites []model.Website

	// 先去掉注释
	noComments := stripNginxComments(content)

	blocks := extractServerBlocks(noComments, defaultName)

	for _, b := range blocks {
		site := extractSiteFromServerBlock(noComments[b.start:b.end], b.filename, defaultName)
		if site != nil {
			sites = append(sites, *site)
		}
	}
	return sites
}

// expandNginxMergedConfig 将 nginx -T 输出按 # configuration file 注释拆分，
// 解决嵌套 include 中 server 块归属问题
func expandNginxMergedConfig(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	confRe := regexp.MustCompile(`^#\s*configuration file\s+(\S+):`)
	var currentFile string
	var sb strings.Builder
	flush := func() {
		if currentFile != "" {
			result[currentFile] = sb.String()
		} else {
			result["__main__"] = sb.String()
		}
		sb.Reset()
	}
	for _, line := range lines {
		if m := confRe.FindStringSubmatch(line); len(m) >= 2 {
			flush()
			currentFile = m[1]
			continue
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	flush()
	return result
}

// stripNginxComments 去掉 # 后的行尾注释（保留引号内的 #）
func stripNginxComments(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			sb.WriteByte(c)
			sb.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			sb.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			sb.WriteByte(c)
			continue
		}
		if c == '#' && !inSingle && !inDouble {
			// 跳过到行尾
			for i < len(s) && s[i] != '\n' {
				i++
			}
			if i < len(s) {
				sb.WriteByte('\n')
			}
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// extractServerBlocks 提取所有 server { ... } 块的位置
func extractServerBlocks(content string, defaultFile string) []struct {
	start, end int
	filename   string
} {
	var result []struct {
		start, end int
		filename   string
	}
	serverRe := regexp.MustCompile(`(?is)server\s*\{`)
	locs := serverRe.FindAllStringIndex(content, -1)
	for _, loc := range locs {
		openIdx := loc[1] - 1
		depth := 1
		pos := openIdx + 1
		inSingle := false
		inDouble := false
		for pos < len(content) {
			c := content[pos]
			if c == '\\' && pos+1 < len(content) {
				pos += 2
				continue
			}
			if c == '\'' && !inDouble {
				inSingle = !inSingle
			} else if c == '"' && !inSingle {
				inDouble = !inDouble
			} else if !inSingle && !inDouble {
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						result = append(result, struct {
							start, end int
							filename   string
						}{start: loc[0], end: pos + 1, filename: defaultFile})
						break
					}
				}
			}
			pos++
		}
	}
	return result
}

// extractSiteFromServerBlock 从 server 块内容中提取 site 信息
func extractSiteFromServerBlock(block, fileName, defaultName string) *model.Website {
	if fileName == "" {
		fileName = defaultName
		if fileName == "nginx" {
			fileName = "default"
		}
	}
	if fileName == "" {
		fileName = "default"
	}

	// 收集 server_name
	serverName := ""
	serverNameRe := regexp.MustCompile(`(?m)server_name\s+([^;]+);`)
	if m := serverNameRe.FindStringSubmatch(block); len(m) >= 2 {
		names := strings.Fields(m[1])
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

	// 收集 listen 端口
	var listenPorts []int
	hasSSL := false
	listenRe := regexp.MustCompile(`(?m)listen\s+([^;]+);`)
	listenMatches := listenRe.FindAllStringSubmatch(block, -1)
	for _, m := range listenMatches {
		if len(m) < 2 {
			continue
		}
		args := strings.Fields(m[1])
		port := 0
		defaultPort := false
		ssl := false
		for _, a := range args {
			if a == "default_server" {
				defaultPort = true
			}
			if a == "ssl" {
				ssl = true
			}
			if p, err := strconv.Atoi(a); err == nil {
				port = p
			}
		}
		if port == 0 && defaultPort {
			port = 80
		}
		if port > 0 {
			listenPorts = append(listenPorts, port)
			if ssl {
				hasSSL = true
			}
		}
	}
	if len(listenPorts) == 0 {
		// 没有 listen 指令的 server 块跳过
		return nil
	}

	// root
	rootDir := ""
	if m := regexp.MustCompile(`(?m)root\s+([^;]+);`).FindStringSubmatch(block); len(m) >= 2 {
		rootDir = strings.TrimSpace(m[1])
	}

	// proxy_pass
	proxyPass := ""
	if m := regexp.MustCompile(`(?m)proxy_pass\s+([^;]+);`).FindStringSubmatch(block); len(m) >= 2 {
		proxyPass = strings.TrimSpace(m[1])
	}

	if serverName == "" || serverName == "_" {
		serverName = fileName
		if serverName == "nginx" {
			serverName = "default"
		}
	}
	if serverName == "" {
		serverName = "default"
	}

	port := listenPorts[0]
	// 如果同时有 80 和 443，优先 80
	if hasSSL {
		for _, p := range listenPorts {
			if p == 80 || p == 8080 {
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

	return &model.Website{
		Name:        name,
		Domain:      serverName,
		Port:        port,
		Root:        rootDir,
		Type:        siteType,
		ProxyTarget: proxyPass,
		SSL:         hasSSL,
		Enabled:     true,
		Managed:     false,
		ConfigFile:  fileName,
		Remark:      "检测到外部创建的网站",
	}
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

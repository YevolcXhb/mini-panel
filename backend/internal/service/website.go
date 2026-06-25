package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	_ = s.removeConfig(w)
	return s.repo.Delete(id)
}

func (s *WebsiteService) GetByID(id uint) (*model.Website, error) {
	return s.repo.GetByID(id)
}

func (s *WebsiteService) List() ([]model.Website, error) {
	return s.repo.List()
}

func (s *WebsiteService) ToggleEnable(id uint, enabled bool) error {
	w, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	w.Enabled = enabled
	if err := s.repo.Update(w); err != nil {
		return err
	}
	return s.applyConfig(w)
}

func (s *WebsiteService) ReloadNginx() error {
	if !syscmd.Which("nginx") {
		return fmt.Errorf("nginx is not installed")
	}
	output, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx config test failed: %s", string(output))
	}
	reloadCmd := exec.Command("nginx", "-s", "reload")
	if _, err := reloadCmd.CombinedOutput(); err != nil {
		_ = exec.Command("systemctl", "reload", "nginx").Run()
	}
	return nil
}

func (s *WebsiteService) GetNginxConfigDir() string {
	candidates := []string{
		"/etc/nginx/conf.d",
		"/etc/nginx/sites-enabled",
		"/usr/local/nginx/conf/conf.d",
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if testWritable(dir) {
				return dir
			}
		}
	}
	panelDir := filepath.Join(global.GetDataDir(), "nginx")
	_ = os.MkdirAll(panelDir, 0755)
	return panelDir
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

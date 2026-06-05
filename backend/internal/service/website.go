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
	cmd := exec.Command("nginx", "-t")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nginx config test failed: %s", string(out))
	}
	cmd = exec.Command("nginx", "-s", "reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nginx reload failed: %s", string(out))
	}
	return nil
}

func (s *WebsiteService) nginxConfPath(w *model.Website) string {
	return filepath.Join(global.GetDataDir(), "nginx", fmt.Sprintf("%s.conf", w.Domain))
}

func (s *WebsiteService) removeConfig(w *model.Website) error {
	path := s.nginxConfPath(w)
	_ = os.Remove(path)
	return nil
}

func (s *WebsiteService) applyConfig(w *model.Website) error {
	if !w.Enabled {
		return s.removeConfig(w)
	}

	confDir := filepath.Join(global.GetDataDir(), "nginx")
	_ = os.MkdirAll(confDir, 0755)

	var sb strings.Builder
	port := w.Port
	if port == 0 {
		port = 80
	}

	if w.SSL {
		sb.WriteString(fmt.Sprintf("server {\n  listen %d;\n  server_name %s;\n  return 301 https://$server_name$request_uri;\n}\n\n", port, w.Domain))
		sb.WriteString(fmt.Sprintf("server {\n  listen %d ssl;\n  server_name %s;\n", port+443, w.Domain))
		if w.SSLCert != "" {
			sb.WriteString(fmt.Sprintf("  ssl_certificate %s;\n", w.SSLCert))
		}
		if w.SSLKey != "" {
			sb.WriteString(fmt.Sprintf("  ssl_certificate_key %s;\n", w.SSLKey))
		}
	} else {
		sb.WriteString(fmt.Sprintf("server {\n  listen %d;\n  server_name %s;\n", port, w.Domain))
	}

	if w.Type == "proxy" && w.ProxyTarget != "" {
		sb.WriteString(fmt.Sprintf("  location / {\n    proxy_pass %s;\n    proxy_set_header Host $host;\n    proxy_set_header X-Real-IP $remote_addr;\n  }\n", w.ProxyTarget))
	} else {
		root := w.Root
		if root == "" {
			root = filepath.Join(global.GetDataDir(), "www", w.Domain)
			_ = os.MkdirAll(root, 0755)
		}
		sb.WriteString(fmt.Sprintf("  root %s;\n  index index.html index.htm;\n  location / {\n    try_files $uri $uri/ =404;\n  }\n", root))
	}

	sb.WriteString("}\n")

	path := s.nginxConfPath(w)
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return err
	}

	_ = s.ReloadNginx()
	return nil
}

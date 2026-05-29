package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
)

type AppService struct {
	appRepo    *repository.AppRepository
	instRepo   *repository.AppInstallRepository
	sourceRepo *repository.AppSourceRepository
	ctnService *ContainerService
}

func NewAppService() *AppService {
	return &AppService{
		appRepo:    repository.NewAppRepository(global.DB),
		instRepo:   repository.NewAppInstallRepository(global.DB),
		sourceRepo: repository.NewAppSourceRepository(global.DB),
		ctnService: NewContainerService(),
	}
}

func (s *AppService) List() ([]model.App, error) {
	return s.appRepo.List()
}

func (s *AppService) Installed() ([]model.AppInstall, error) {
	return s.instRepo.List()
}

func (s *AppService) Install(req dto.AppInstallRequest) (*model.AppInstall, error) {
	app, err := s.appRepo.GetByID(req.AppID)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	inst := &model.AppInstall{
		AppID:     req.AppID,
		Name:      req.Name,
		Status:    "installing",
		Image:     app.Image,
		Container: req.Name,
		Port:      req.Port,
		Path:      filepath.Join(global.GetDataDir(), "apps", req.Name),
	}
	if err := s.instRepo.Create(inst); err != nil {
		return nil, err
	}

	var envs []string
	if app.EnvVars != "" {
		var envMap map[string]string
		if err := json.Unmarshal([]byte(app.EnvVars), &envMap); err == nil {
			for k, v := range envMap {
				envs = append(envs, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}
	if req.Port > 0 {
		envs = append(envs, fmt.Sprintf("PORT=%d", req.Port))
	}

	var volumes []string
	if app.Volumes != "" {
		var volMap map[string]string
		if err := json.Unmarshal([]byte(app.Volumes), &volMap); err == nil {
			for k, v := range volMap {
				hostPath := filepath.Join(inst.Path, k)
				os.MkdirAll(hostPath, 0755)
				volumes = append(volumes, fmt.Sprintf("%s:%s", hostPath, v))
			}
		}
	}

	if s.ctnService.IsAvailable() {
		if err := s.ctnService.client.Pull(app.Image, req.Name); err != nil {
			inst.Status = "failed"
			s.instRepo.Update(inst)
			return inst, fmt.Errorf("pull image: %w", err)
		}
		if err := s.ctnService.client.Run(req.Name, true, envs, volumes); err != nil {
			inst.Status = "failed"
			s.instRepo.Update(inst)
			return inst, fmt.Errorf("run container: %w", err)
		}
	} else {
		inst.Status = "not_supported"
		s.instRepo.Update(inst)
		return inst, fmt.Errorf("dockroot not available")
	}

	inst.Status = "running"
	s.instRepo.Update(inst)
	return inst, nil
}

func (s *AppService) Uninstall(id uint) error {
	inst, err := s.instRepo.GetByID(id)
	if err != nil {
		return err
	}
	if s.ctnService.IsAvailable() {
		_ = s.ctnService.client.Rm(inst.Container)
	}
	_ = os.RemoveAll(inst.Path)
	return s.instRepo.Delete(id)
}

func (s *AppService) InitDefaultApps() error {
	apps := []model.App{
		{
			Name:        "Nginx",
			Image:       "docker.io/library/nginx:alpine",
			Description: "High performance web server",
			Category:    "web",
			Version:     "1.25",
			Icon:        "nginx",
			EnvVars:     `{"NGINX_PORT":"80"}`,
			Volumes:     `{"html":"/usr/share/nginx/html","conf":"/etc/nginx/conf.d"}`,
		},
		{
			Name:        "MySQL",
			Image:       "docker.io/library/mysql:8.0",
			Description: "Relational database",
			Category:    "database",
			Version:     "8.0",
			Icon:        "mysql",
			EnvVars:     `{"MYSQL_ROOT_PASSWORD":"root"}`,
			Volumes:     `{"data":"/var/lib/mysql"}`,
		},
		{
			Name:        "Redis",
			Image:       "docker.io/library/redis:alpine",
			Description: "In-memory data store",
			Category:    "database",
			Version:     "7.2",
			Icon:        "redis",
			EnvVars:     `{}`,
			Volumes:     `{"data":"/data"}`,
		},
		{
			Name:        "Alpine",
			Image:       "docker.io/library/alpine:latest",
			Description: "Minimal Linux container",
			Category:    "system",
			Version:     "latest",
			Icon:        "linux",
			EnvVars:     `{}`,
			Volumes:     `{}`,
		},
	}
	for _, app := range apps {
		existing, _ := s.appRepo.GetByID(app.ID)
		if existing == nil {
			s.appRepo.Create(&app)
		}
	}
	return nil
}

func (s *AppService) ListSources() ([]model.AppSource, error) {
	return s.sourceRepo.List()
}

func (s *AppService) AddSource(name, url string) error {
	source := &model.AppSource{
		Name:    name,
		URL:     url,
		Enabled: true,
	}
	return s.sourceRepo.Create(source)
}

func (s *AppService) RemoveSource(id uint) error {
	return s.sourceRepo.Delete(id)
}

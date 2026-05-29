package service

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
)

type AppService struct {
	appRepo    *repository.AppRepository
	detailRepo *repository.AppDetailRepository
	instRepo   *repository.AppInstallRepository
	sourceRepo *repository.AppSourceRepository
	ctnService *ContainerService
}

func NewAppService() *AppService {
	return &AppService{
		appRepo:    repository.NewAppRepository(global.DB),
		detailRepo: repository.NewAppDetailRepository(global.DB),
		instRepo:   repository.NewAppInstallRepository(global.DB),
		sourceRepo: repository.NewAppSourceRepository(global.DB),
		ctnService: NewContainerService(),
	}
}

func (s *AppService) List(category string) ([]model.App, error) {
	if category != "" && category != "all" {
		return s.appRepo.ListByCategory(category)
	}
	return s.appRepo.List()
}

func (s *AppService) Search(keyword string) ([]model.App, error) {
	return s.appRepo.Search(keyword)
}

func (s *AppService) GetWithDetails(id uint) (*model.App, []model.AppDetail, error) {
	app, err := s.appRepo.GetByID(id)
	if err != nil {
		return nil, nil, err
	}
	details, err := s.detailRepo.ListByAppID(id)
	if err != nil {
		return nil, nil, err
	}
	return app, details, nil
}

func (s *AppService) Installed() ([]model.AppInstall, error) {
	return s.instRepo.List()
}

func (s *AppService) Install(req dto.AppInstallRequest) (*model.AppInstall, error) {
	app, err := s.appRepo.GetByID(req.AppID)
	if err != nil {
		return nil, fmt.Errorf("app not found: %w", err)
	}

	var detail *model.AppDetail
	if req.AppDetailID > 0 {
		detail, err = s.detailRepo.GetByID(req.AppDetailID)
		if err != nil {
			return nil, fmt.Errorf("version not found: %w", err)
		}
	} else {
		details, _ := s.detailRepo.ListByAppID(req.AppID)
		if len(details) > 0 {
			detail = &details[0]
		}
	}

	image := app.Key
	version := "latest"
	if detail != nil {
		image = detail.Image
		version = detail.Version
	}

	inst := &model.AppInstall{
		AppID:       req.AppID,
		AppDetailID: req.AppDetailID,
		Name:        req.Name,
		Status:      "installing",
		Image:       image,
		Version:     version,
		Container:   req.Name,
		Port:        req.Port,
		Path:        filepath.Join(global.GetDataDir(), "apps", req.Name),
	}
	if err := s.instRepo.Create(inst); err != nil {
		return nil, err
	}

	var envs []string
	if detail != nil && detail.EnvVars != "" {
		var envMap map[string]string
		if err := json.Unmarshal([]byte(detail.EnvVars), &envMap); err == nil {
			for k, v := range envMap {
				envs = append(envs, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}
	if req.Env != nil {
		for k, v := range req.Env {
			envs = append(envs, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if req.Port > 0 {
		envs = append(envs, fmt.Sprintf("PORT=%d", req.Port))
	}

	var volumes []string
	if detail != nil && detail.Volumes != "" {
		var volMap map[string]string
		if err := json.Unmarshal([]byte(detail.Volumes), &volMap); err == nil {
			for k, v := range volMap {
				hostPath := filepath.Join(inst.Path, k)
				os.MkdirAll(hostPath, 0755)
				volumes = append(volumes, fmt.Sprintf("%s:%s", hostPath, v))
			}
		}
	}
	if req.Volumes != nil {
		for k, v := range req.Volumes {
			hostPath := filepath.Join(inst.Path, k)
			os.MkdirAll(hostPath, 0755)
			volumes = append(volumes, fmt.Sprintf("%s:%s", hostPath, v))
		}
	}

	if s.ctnService.IsAvailable() {
		if err := s.ctnService.client.Pull(image, req.Name); err != nil {
			inst.Status = "failed"
			inst.Message = err.Error()
			s.instRepo.Update(inst)
			return inst, fmt.Errorf("pull image: %w", err)
		}
		if err := s.ctnService.client.Run(req.Name, true, envs, volumes); err != nil {
			inst.Status = "failed"
			inst.Message = err.Error()
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

func (s *AppService) SyncFromRemote(sourceID uint) error {
	source, err := s.sourceRepo.GetByID(sourceID)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	if strings.HasSuffix(source.URL, ".zip") {
		return s.syncFrom1PanelZip(source)
	}
	return s.syncFromJSON(source)
}

func (s *AppService) syncFromJSON(source *model.AppSource) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(source.URL)
	if err != nil {
		return fmt.Errorf("fetch remote apps: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var remoteApps []dto.RemoteApp
	if err := json.Unmarshal(body, &remoteApps); err != nil {
		return fmt.Errorf("parse apps json: %w", err)
	}

	for _, ra := range remoteApps {
		existing, _ := s.appRepo.GetByKey(ra.Key)
		if existing != nil {
			continue
		}
		app := &model.App{
			Key:         ra.Key,
			Name:        ra.Name,
			Description: ra.Description,
			ShortDesc:   ra.ShortDesc,
			Icon:        ra.Icon,
			Category:    ra.Category,
			Type:        ra.Type,
			Status:      "active",
			Website:     ra.Website,
			Github:      ra.Github,
			Document:    ra.Document,
			Resource:    "remote",
			SourceID:    source.ID,
		}
		if app.Type == "" {
			app.Type = "container"
		}
		if err := s.appRepo.Create(app); err != nil {
			continue
		}

		for _, rv := range ra.Versions {
			detail := &model.AppDetail{
				AppID:   app.ID,
				Version: rv.Version,
				Image:   rv.Image,
				EnvVars: rv.EnvVars,
				Volumes: rv.Volumes,
				Command: rv.Command,
				Params:  rv.Params,
				Status:  "active",
			}
			_ = s.detailRepo.Create(detail)
		}
	}

	return nil
}

type panelAppList struct {
	Valid        bool          `json:"valid"`
	LastModified int           `json:"lastModified"`
	Apps         []panelAppDef `json:"apps"`
}

type panelAppDef struct {
	Icon         string            `json:"icon"`
	Name         string            `json:"name"`
	ReadMe       string            `json:"readMe"`
	LastModified int               `json:"lastModified"`
	AppProperty  panelAppProperty  `json:"additionalProperties"`
	Versions     []panelAppVersion `json:"versions"`
}

type panelAppProperty struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Tags        []string `json:"tags"`
	ShortDescZh string   `json:"shortDescZh"`
	ShortDescEn string   `json:"shortDescEn"`
	Description struct {
		Zh string `json:"zh"`
		En string `json:"en"`
	} `json:"description"`
	Key      string `json:"key"`
	Website  string `json:"website"`
	Github   string `json:"github"`
	Document string `json:"document"`
}

type panelAppVersion struct {
	Name         string `json:"name"`
	LastModified int    `json:"lastModified"`
	DownloadURL  string `json:"downloadUrl"`
}

func (s *AppService) syncFrom1PanelZip(source *model.AppSource) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(source.URL)
	if err != nil {
		return fmt.Errorf("download zip: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote returned status %d", resp.StatusCode)
	}

	tmpDir := filepath.Join(os.TempDir(), "minipanel-apps")
	os.MkdirAll(tmpDir, 0755)
	zipPath := filepath.Join(tmpDir, "1panel.json.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return fmt.Errorf("save zip: %w", err)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	var list panelAppList
	for _, file := range zr.File {
		if file.Name != "1panel.json" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &list); err != nil {
			return fmt.Errorf("parse 1panel.json: %w", err)
		}
		break
	}

	for _, pa := range list.Apps {
		key := pa.AppProperty.Key
		if key == "" {
			key = pa.Name
		}
		existing, _ := s.appRepo.GetByKey(key)
		if existing != nil {
			continue
		}

		desc := pa.AppProperty.Description.Zh
		if desc == "" {
			desc = pa.AppProperty.Description.En
		}
		shortDesc := pa.AppProperty.ShortDescZh
		if shortDesc == "" {
			shortDesc = pa.AppProperty.ShortDescEn
		}
		category := "other"
		if len(pa.AppProperty.Tags) > 0 {
			category = pa.AppProperty.Tags[0]
		}

		app := &model.App{
			Key:         key,
			Name:        pa.AppProperty.Name,
			Description: desc,
			ShortDesc:   shortDesc,
			Category:    category,
			Type:        pa.AppProperty.Type,
			Status:      "active",
			Website:     pa.AppProperty.Website,
			Github:      pa.AppProperty.Github,
			Document:    pa.AppProperty.Document,
			Resource:    "remote",
			SourceID:    source.ID,
		}
		if app.Type == "" {
			app.Type = "container"
		}
		if err := s.appRepo.Create(app); err != nil {
			continue
		}

		for _, pv := range pa.Versions {
			image := s.extractImageFrom1Panel(pv.DownloadURL)
			detail := &model.AppDetail{
				AppID:   app.ID,
				Version: pv.Name,
				Image:   image,
				Status:  "active",
			}
			_ = s.detailRepo.Create(detail)
		}
	}

	return nil
}

func (s *AppService) extractImageFrom1Panel(downloadURL string) string {
	if downloadURL == "" {
		return ""
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	tmpDir := filepath.Join(os.TempDir(), "minipanel-app-version")
	os.MkdirAll(tmpDir, 0755)
	zipPath := filepath.Join(tmpDir, fmt.Sprintf("app-%d.zip", time.Now().Unix()))
	f, err := os.Create(zipPath)
	if err != nil {
		return ""
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return ""
	}
	defer os.Remove(zipPath)

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return ""
	}
	defer zr.Close()

	for _, file := range zr.File {
		if !strings.HasSuffix(file.Name, "docker-compose.yml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		var compose struct {
			Services map[string]struct {
				Image string `json:"image" yaml:"image"`
			} `json:"services" yaml:"services"`
		}
		if err := json.Unmarshal(data, &compose); err == nil {
			for _, svc := range compose.Services {
				if svc.Image != "" {
					return svc.Image
				}
			}
		}
		break
	}
	return ""
}

func (s *AppService) InitDefaultApps() error {
	apps := []model.App{
		{Key: "nginx", Name: "Nginx", Description: "High performance web server", Category: "web", Icon: "nginx"},
		{Key: "mysql", Name: "MySQL", Description: "Popular open source database", Category: "database", Icon: "mysql"},
		{Key: "redis", Name: "Redis", Description: "In-memory data store", Category: "database", Icon: "redis"},
		{Key: "postgres", Name: "PostgreSQL", Description: "Advanced open source database", Category: "database", Icon: "postgres"},
	}
	for _, app := range apps {
		app.Type = "container"
		app.Status = "active"
		app.Resource = "builtin"
		existing, _ := s.appRepo.GetByKey(app.Key)
		if existing == nil {
			_ = s.appRepo.Create(&app)
		}
	}
	return nil
}

func (s *AppService) InitDefaultSource() error {
	sources, _ := s.sourceRepo.List()
	for _, src := range sources {
		if src.Name == "1Panel官方商店" {
			return nil
		}
	}
	return s.sourceRepo.Create(&model.AppSource{
		Name:    "1Panel官方商店",
		URL:     "https://apps-assets.fit2cloud.com/stable/1panel.json.zip",
		Enabled: true,
	})
}

func (s *AppService) ListSources() ([]model.AppSource, error) {
	return s.sourceRepo.List()
}

func (s *AppService) AddSource(name, url string) (*model.AppSource, error) {
	source := &model.AppSource{Name: name, URL: url, Enabled: true}
	return source, s.sourceRepo.Create(source)
}

func (s *AppService) RemoveSource(id uint) error {
	return s.sourceRepo.Delete(id)
}

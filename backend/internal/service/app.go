package service

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	"gopkg.in/yaml.v3"
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
	global.LOG.Infof("[Install] start install app_id=%d detail_id=%d name=%s port=%d", req.AppID, req.AppDetailID, req.Name, req.Port)
	defer global.LOG.Infof("[Install] finish install name=%s", req.Name)

	app, err := s.appRepo.GetByID(req.AppID)
	if err != nil {
		global.LOG.Errorf("[Install] get app failed: %v", err)
		return nil, fmt.Errorf("app not found: %w", err)
	}
	global.LOG.Infof("[Install] app found key=%s name=%s", app.Key, app.Name)

	var detail *model.AppDetail
	if req.AppDetailID > 0 {
		detail, err = s.detailRepo.GetByID(req.AppDetailID)
		if err != nil {
			global.LOG.Errorf("[Install] get detail failed: %v", err)
			return nil, fmt.Errorf("version not found: %w", err)
		}
		global.LOG.Infof("[Install] detail found image=%s version=%s", detail.Image, detail.Version)
	} else {
		details, _ := s.detailRepo.ListByAppID(req.AppID)
		if len(details) > 0 {
			detail = &details[0]
			global.LOG.Infof("[Install] using first detail image=%s version=%s", detail.Image, detail.Version)
		} else {
			global.LOG.Warnf("[Install] no detail found for app_id=%d", req.AppID)
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
		global.LOG.Errorf("[Install] create install record failed: %v", err)
		return nil, err
	}
	global.LOG.Infof("[Install] install record created id=%d path=%s", inst.ID, inst.Path)

	// ========== 解析 docker-compose.yml ==========
	var composeEnvs []string
	var composeVolumes []string
	var exposedPort int

	composePath := filepath.Join(inst.Path, "docker-compose.yml")
	if detail != nil && detail.DownloadURL != "" {
		if err := downloadAppPackage(detail.DownloadURL, inst.Path); err != nil {
			global.LOG.Warnf("[Install] download package failed: %v", err)
		}
	}

	if data, err := os.ReadFile(composePath); err == nil {
		var compose struct {
			Services map[string]struct {
				Image       string      `yaml:"image"`
				Ports       []string    `yaml:"ports"`
				Environment interface{} `yaml:"environment"`
				Volumes     []string    `yaml:"volumes"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &compose); err == nil {
			for _, svc := range compose.Services {
				if svc.Image != "" {
					image = svc.Image
					inst.Image = image
				}
				if len(svc.Ports) > 0 {
					exposedPort = extractHostPort(svc.Ports)
				}
				switch env := svc.Environment.(type) {
				case map[string]interface{}:
					for k, v := range env {
						composeEnvs = append(composeEnvs, fmt.Sprintf("%s=%s", k, fmt.Sprintf("%v", v)))
					}
				}
				for _, v := range svc.Volumes {
					parts := strings.Split(v, ":")
					if len(parts) >= 2 {
						hostPath := filepath.Join(inst.Path, filepath.Base(parts[0]))
						os.MkdirAll(hostPath, 0755)
						composeVolumes = append(composeVolumes, fmt.Sprintf("%s:%s", hostPath, parts[1]))
					}
				}
				break
			}
			global.LOG.Infof("[Install] compose parsed image=%s port=%d envs=%d volumes=%d", image, exposedPort, len(composeEnvs), len(composeVolumes))
		} else {
			global.LOG.Warnf("[Install] unmarshal compose failed: %v", err)
		}
	} else {
		global.LOG.Infof("[Install] no docker-compose.yml found at %s", composePath)
	}

	// 端口优先级：用户指定 > compose 提取 > detail 默认值
	if req.Port > 0 {
		inst.Port = req.Port
	} else if exposedPort > 0 {
		inst.Port = exposedPort
	}

	var envs []string
	envSet := make(map[string]string)
	for _, e := range composeEnvs {
		if idx := strings.Index(e, "="); idx >= 0 {
			envSet[e[:idx]] = e[idx+1:]
		}
	}
	if detail != nil && detail.EnvVars != "" {
		var envMap map[string]string
		if err := json.Unmarshal([]byte(detail.EnvVars), &envMap); err == nil {
			for k, v := range envMap {
				envSet[k] = v
			}
			global.LOG.Infof("[Install] detail envVars loaded count=%d", len(envMap))
		} else {
			global.LOG.Warnf("[Install] unmarshal detail envVars failed: %v", err)
		}
	}
	if req.Env != nil {
		for k, v := range req.Env {
			envSet[k] = v
		}
		global.LOG.Infof("[Install] user env count=%d", len(req.Env))
	}
	for k, v := range envSet {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}
	if inst.Port > 0 {
		envs = append(envs, fmt.Sprintf("PORT=%d", inst.Port))
	}

	var volumes []string
	volumes = append(volumes, composeVolumes...)
	if detail != nil && detail.Volumes != "" {
		var volMap map[string]string
		if err := json.Unmarshal([]byte(detail.Volumes), &volMap); err == nil {
			for k, v := range volMap {
				hostPath := filepath.Join(inst.Path, k)
				os.MkdirAll(hostPath, 0755)
				volumes = append(volumes, fmt.Sprintf("%s:%s", hostPath, v))
			}
			global.LOG.Infof("[Install] detail volumes loaded count=%d", len(volMap))
		} else {
			global.LOG.Warnf("[Install] unmarshal detail volumes failed: %v", err)
		}
	}
	if req.Volumes != nil {
		for k, v := range req.Volumes {
			hostPath := filepath.Join(inst.Path, k)
			os.MkdirAll(hostPath, 0755)
			volumes = append(volumes, fmt.Sprintf("%s:%s", hostPath, v))
		}
		global.LOG.Infof("[Install] user volumes count=%d", len(req.Volumes))
	}

	global.LOG.Infof("[Install] final params image=%s container=%s port=%d envs=%d volumes=%d", image, req.Name, req.Port, len(envs), len(volumes))

	if s.ctnService.IsAvailable() {
		global.LOG.Infof("[Install] pulling image %s ...", image)
		if err := s.ctnService.client.Pull(image, req.Name); err != nil {
			global.LOG.Errorf("[Install] pull image failed: %v", err)
			inst.Status = "failed"
			inst.Message = err.Error()
			s.instRepo.Update(inst)
			return inst, fmt.Errorf("pull image: %w", err)
		}
		global.LOG.Infof("[Install] pull image success")

		global.LOG.Infof("[Install] running container %s ...", req.Name)
		if err := s.ctnService.client.Run(req.Name, true, envs, volumes); err != nil {
			global.LOG.Errorf("[Install] run container failed: %v", err)
			inst.Status = "failed"
			inst.Message = err.Error()
			s.instRepo.Update(inst)
			return inst, fmt.Errorf("run container: %w", err)
		}
		global.LOG.Infof("[Install] run container success")
	} else {
		global.LOG.Errorf("[Install] dockroot not available")
		inst.Status = "not_supported"
		s.instRepo.Update(inst)
		return inst, fmt.Errorf("dockroot not available")
	}

	inst.Status = "running"
	s.instRepo.Update(inst)
	global.LOG.Infof("[Install] install success id=%d status=running", inst.ID)
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
				AppID:       app.ID,
				Version:     pv.Name,
				Image:       image,
				DownloadURL: pv.DownloadURL,
				Status:      "active",
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
				Image string `yaml:"image"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &compose); err == nil {
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

// extractHostPort 从 compose ports 中提取可用端口。
// 优先返回主机端口（如 "8080:80" 返回 8080）；
// 若主机端口是变量（如 "${VAR}:3306"），则返回容器端口（3306）作为兜底。
func extractHostPort(ports []string) int {
	for _, p := range ports {
		p = strings.TrimSpace(p)
		idx := strings.LastIndex(p, ":")
		if idx < 0 {
			if port, err := strconv.Atoi(p); err == nil {
				return port
			}
			continue
		}
		hostPart := strings.TrimSpace(p[:idx])
		containerPart := strings.TrimSpace(p[idx+1:])

		hostClean := strings.TrimPrefix(strings.TrimPrefix(hostPart, "$"), "{")
		hostClean = strings.TrimSuffix(hostClean, "}")
		if port, err := strconv.Atoi(hostClean); err == nil {
			return port
		}
		if port, err := strconv.Atoi(containerPart); err == nil {
			return port
		}
	}
	return 0
}

// downloadAppPackage 下载 1Panel 版本包并解压到目标目录
func downloadAppPackage(url, destDir string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	os.MkdirAll(destDir, 0755)
	zipPath := filepath.Join(destDir, "app.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	defer os.Remove(zipPath)

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	for _, file := range zr.File {
		fpath := filepath.Join(destDir, file.Name)
		if file.FileInfo().IsDir() {
			os.MkdirAll(fpath, file.Mode())
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), 0755)
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			continue
		}
		io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
	}
	return nil
}

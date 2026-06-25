package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	global.LOG.Infof("[Install] start install app_id=%d detail_id=%d name=%s", req.AppID, req.AppDetailID, req.Name)
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

	// lazy extract/resolve image if empty or contains variables
	if detail != nil && detail.DownloadURL != "" && (detail.Image == "" || strings.Contains(detail.Image, "$")) {
		global.LOG.Infof("[Install] detail image empty or contains vars, extracting from %s", detail.DownloadURL)
		detail.Image = s.extractImageFrom1Panel(detail.DownloadURL)
		if detail.Image != "" {
			_ = s.detailRepo.Update(detail)
			global.LOG.Infof("[Install] extracted and saved image=%s", detail.Image)
		}
	}

	image := app.Key
	version := "latest"
	if detail != nil {
		if detail.Image != "" {
			image = detail.Image
		}
		version = detail.Version
	}

	instName := strings.TrimSpace(req.Name)
	if instName == "" {
		instName = app.Key
	}

	inst := &model.AppInstall{
		AppID:       req.AppID,
		AppDetailID: req.AppDetailID,
		Name:        instName,
		Status:      "installing",
		Image:       image,
		Version:     version,
		Container:   instName,
		Path:        filepath.Join(global.GetDataDir(), "apps", instName),
	}

	existing, _ := s.instRepo.GetByName(instName)
	if existing != nil {
		if existing.Status == "running" {
			return nil, fmt.Errorf("应用 %s 已存在且正在运行，请更换实例名称", instName)
		}
		global.LOG.Infof("[Install] found existing install record id=%d status=%s, reusing", existing.ID, existing.Status)
		inst.ID = existing.ID
		inst.CreatedAt = existing.CreatedAt
		if err := s.instRepo.Update(inst); err != nil {
			global.LOG.Errorf("[Install] update existing install record failed: %v", err)
			return nil, err
		}
	} else {
		if err := s.instRepo.Create(inst); err != nil {
			global.LOG.Errorf("[Install] create install record failed: %v", err)
			return nil, err
		}
		global.LOG.Infof("[Install] install record created id=%d path=%s", inst.ID, inst.Path)
	}

	// ========== 解析 docker-compose.yml ==========
	var composeEnvs []string
	var composeVolumes []string
	var containerPort int

	composePath := filepath.Join(inst.Path, "docker-compose.yml")
	if detail != nil && detail.DownloadURL != "" {
		if err := downloadAppPackage(detail.DownloadURL, inst.Path); err != nil {
			global.LOG.Warnf("[Install] download package failed: %v", err)
		}
	}

	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		composePath = findComposeFile(inst.Path)
		global.LOG.Infof("[Install] searching compose file, found: %s", composePath)
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
			dotEnv := parseDotFile(filepath.Dir(composePath))
			dataJSONImage := parseDataJSON(filepath.Dir(composePath))
			scanEnv := scanAllEnvFiles(filepath.Dir(composePath))
			for k, v := range scanEnv {
				if _, exists := dotEnv[k]; !exists {
					dotEnv[k] = v
				}
			}
			for _, svc := range compose.Services {
				if svc.Image != "" && strings.Contains(svc.Image, "$") {
					varNames := extractVarNames(svc.Image)
					for _, vn := range varNames {
						if dataJSONImage != "" {
							dotEnv[vn] = dataJSONImage
						}
					}
				}
				break
			}
			global.LOG.Infof("[Install] env sources: dotEnv=%d, dataJSON=%s, scanEnv=%d", len(dotEnv), dataJSONImage, len(scanEnv))
			for _, svc := range compose.Services {
				if svc.Image != "" {
					resolved := resolveEnvVars(svc.Image, dotEnv)
					if resolved != svc.Image && !strings.Contains(resolved, "$") {
						image = resolved
					} else if dataJSONImage != "" && !strings.Contains(dataJSONImage, "$") {
						image = dataJSONImage
					}
					if !strings.Contains(image, "$") {
						inst.Image = image
					}
				}
				if containerPort == 0 {
					containerPort = extractContainerPort(svc.Ports)
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
			global.LOG.Infof("[Install] compose parsed image=%s envs=%d volumes=%d", image, len(composeEnvs), len(composeVolumes))
		} else {
			global.LOG.Warnf("[Install] unmarshal compose failed: %v", err)
		}
	} else {
		global.LOG.Infof("[Install] no docker-compose.yml found at %s", composePath)
	}

	if containerPort > 0 {
		inst.Port = containerPort
	}

	var envs []string
	envSet := make(map[string]string)
	for _, e := range composeEnvs {
		if idx := strings.Index(e, "="); idx >= 0 {
			key := e[:idx]
			val := e[idx+1:]
			if strings.Contains(val, "${") || strings.Contains(val, "$") && strings.IndexByte(val, '$') == 0 {
				global.LOG.Infof("[Install] skip env with unresolved var: %s", e)
				continue
			}
			envSet[key] = val
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

	global.LOG.Infof("[Install] final params image=%s container=%s envs=%d volumes=%d", image, instName, len(envs), len(volumes))

	if strings.Contains(image, "$") {
		global.LOG.Errorf("[Install] image contains unresolved variables: %s", image)
		inst.Status = "failed"
		inst.Message = fmt.Sprintf("镜像名包含未解析的变量: %s，该应用可能需要在 1Panel 中安装", image)
		s.instRepo.Update(inst)
		return inst, fmt.Errorf("image contains unresolved variables: %s", image)
	}

	if s.ctnService.IsAvailable() {
		global.LOG.Infof("[Install] pulling image %s ...", image)
		if err := s.ctnService.client.Pull(image, instName); err != nil {
			global.LOG.Errorf("[Install] pull image failed: %v", err)
			inst.Status = "failed"
			inst.Message = err.Error()
			s.instRepo.Update(inst)
			return inst, fmt.Errorf("pull image: %w", err)
		}
		global.LOG.Infof("[Install] pull image success")

		global.LOG.Infof("[Install] running container %s ...", instName)
		if err := s.ctnService.client.Run(instName, true, envs, volumes); err != nil {
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
	global.LOG.Infof("[Sync] downloading app list from %s", source.URL)
	client := &http.Client{Timeout: 180 * time.Second}
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
			detail := &model.AppDetail{
				AppID:       app.ID,
				Version:     pv.Name,
				Image:       "", // lazy extract on install
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
	global.LOG.Infof("[Sync] downloading package from %s to extract image", downloadURL)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		global.LOG.Warnf("[Sync] download failed: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		global.LOG.Warnf("[Sync] download status %d", resp.StatusCode)
		return ""
	}

	tmpDir := filepath.Join(os.TempDir(), "minipanel-app-version")
	os.MkdirAll(tmpDir, 0755)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("app-%d.pkg", time.Now().Unix()))
	f, err := os.Create(tmpPath)
	if err != nil {
		return ""
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		global.LOG.Warnf("[Sync] save package failed: %v", err)
		return ""
	}
	defer os.Remove(tmpPath)

	// Extract to a temp dir to access .env and data.json in a unified way
	extractDir := filepath.Join(os.TempDir(), fmt.Sprintf("minipanel-app-extract-%d", time.Now().Unix()))
	os.MkdirAll(extractDir, 0755)
	defer os.RemoveAll(extractDir)

	extracted := false
	if extractZip(tmpPath, extractDir) == nil {
		flattenSingleSubdir(extractDir)
		extracted = true
	} else if extractTarGz(tmpPath, extractDir) == nil {
		flattenSingleSubdir(extractDir)
		extracted = true
	}

	if extracted {
		composePath := findComposeFile(extractDir)
		envMap := parseDotFile(filepath.Dir(composePath))
		scanEnv := scanAllEnvFiles(extractDir)
		for k, v := range scanEnv {
			if _, exists := envMap[k]; !exists {
				envMap[k] = v
			}
		}
		dataImage := parseDataJSON(extractDir)

		if composePath != "" {
			if data, err := os.ReadFile(composePath); err == nil {
				var compose struct {
					Services map[string]struct {
						Image string `yaml:"image"`
					} `yaml:"services"`
				}
				if err := yaml.Unmarshal(data, &compose); err == nil {
					for _, svc := range compose.Services {
						if svc.Image == "" {
							continue
						}
						resolved := resolveEnvVars(svc.Image, envMap)
						if !strings.Contains(resolved, "$") {
							global.LOG.Infof("[Sync] extracted image from compose: %s", resolved)
							return resolved
						}
						// Fallback: if image uses a known variable and data.json has image, use it
						if dataImage != "" && !strings.Contains(dataImage, "$") {
							global.LOG.Infof("[Sync] using data.json image: %s", dataImage)
							return dataImage
						}
					}
				}
			}
		}
		if dataImage != "" && !strings.Contains(dataImage, "$") {
			global.LOG.Infof("[Sync] extracted image from data.json: %s", dataImage)
			return dataImage
		}
	}

	// Fallback to streaming extraction for compressed archives
	image := extractImageFromZip(tmpPath)
	envMap := extractEnvFromZip(tmpPath)
	image = resolveEnvVars(image, envMap)
	if image != "" && !strings.Contains(image, "$") {
		global.LOG.Infof("[Sync] extracted image from zip: %s", image)
		return image
	}

	image = extractImageFromTarGz(tmpPath)
	envMap = extractEnvFromTarGz(tmpPath)
	image = resolveEnvVars(image, envMap)
	if image != "" && !strings.Contains(image, "$") {
		global.LOG.Infof("[Sync] extracted image from tar.gz: %s", image)
		return image
	}

	dataImage := extractImageFromDataJSON(tmpPath)
	if dataImage != "" && !strings.Contains(dataImage, "$") {
		global.LOG.Infof("[Sync] extracted image from data.json: %s", dataImage)
		return dataImage
	}

	allEnv := extractAllEnvFromTarGz(tmpPath)
	if len(allEnv) > 0 && image != "" {
		image = resolveEnvVars(image, allEnv)
		if !strings.Contains(image, "$") {
			global.LOG.Infof("[Sync] resolved image from scanned env: %s", image)
			return image
		}
	}

	scriptImage := extractImageFromScripts(tmpPath)
	if scriptImage != "" {
		global.LOG.Infof("[Sync] extracted image from init script: %s", scriptImage)
		return scriptImage
	}

	global.LOG.Warnf("[Sync] failed to extract image from package")
	return ""
}

func extractImageFromZip(path string) string {
	zr, err := zip.OpenReader(path)
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

func extractImageFromTarGz(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return ""
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
		if !strings.HasSuffix(hdr.Name, "docker-compose.yml") {
			continue
		}
		data, err := io.ReadAll(tr)
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

func extractImageFromDataJSON(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return ""
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
		if !strings.HasSuffix(hdr.Name, "data.json") {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			continue
		}
		var appData struct {
			Image    string `json:"image"`
			Versions []struct {
				Name  string `json:"name"`
				Image string `json:"image"`
			} `json:"versions"`
		}
		if err := json.Unmarshal(data, &appData); err == nil {
			if appData.Image != "" {
				return appData.Image
			}
			for _, v := range appData.Versions {
				if v.Image != "" {
					return v.Image
				}
			}
		}
		break
	}
	return ""
}

func extractEnvFromZip(path string) map[string]string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil
	}
	defer zr.Close()

	for _, file := range zr.File {
		if filepath.Base(file.Name) != ".env" {
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
		return parseEnvFile(string(data))
	}
	return nil
}

func extractEnvFromTarGz(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
		if filepath.Base(hdr.Name) != ".env" {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			continue
		}
		return parseEnvFile(string(data))
	}
	return nil
}

func extractAllEnvFromTarGz(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	defer gr.Close()

	envMap := make(map[string]string)
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := strings.ToLower(filepath.Base(hdr.Name))
		if name == "docker-compose.yml" || name == "docker-compose.yaml" || name == "readme.md" || name == "readme.txt" {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			continue
		}
		content := string(data)
		if !strings.Contains(content, "=") {
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "{") || strings.HasPrefix(line, "}") || strings.HasPrefix(line, " ") {
				continue
			}
			idx := strings.Index(line, "=")
			if idx < 0 || idx == 0 {
				continue
			}
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, `"'`)
			if key != "" && val != "" && !strings.Contains(key, " ") {
				envMap[key] = val
			}
		}
	}
	return envMap
}

func parseEnvFile(content string) map[string]string {
	env := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		env[key] = val
	}
	return env
}

func resolveEnvVars(s string, env map[string]string) string {
	if env == nil {
		return s
	}
	for k, v := range env {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
		re := regexp.MustCompile(`\$` + regexp.QuoteMeta(k) + `\b`)
		s = re.ReplaceAllString(s, v)
	}
	return s
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

func (s *AppService) GetIconURL(key string) (string, error) {
	app, err := s.appRepo.GetByKey(key)
	if err != nil {
		return "", err
	}
	return app.Icon, nil
}

func flattenSingleSubdir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return
	}
	subDir := filepath.Join(dir, entries[0].Name())
	files, _ := os.ReadDir(subDir)
	for _, f := range files {
		oldPath := filepath.Join(subDir, f.Name())
		newPath := filepath.Join(dir, f.Name())
		os.Rename(oldPath, newPath)
	}
	os.Remove(subDir)
}

func findComposeFile(dir string) string {
	var found string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if info.Name() == "docker-compose.yml" || info.Name() == "docker-compose.yaml" {
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

func parseDotFile(dir string) map[string]string {
	envMap := make(map[string]string)
	candidates := []string{".env", ".env.dev", ".env.prod", "env"}
	for _, name := range candidates {
		envPath := filepath.Join(dir, name)
		data, err := os.ReadFile(envPath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			idx := strings.Index(line, "=")
			if idx < 0 {
				continue
			}
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, `"'`)
			envMap[key] = val
		}
	}
	return envMap
}

func extractVarNames(s string) []string {
	var names []string
	seen := make(map[string]bool)
	for {
		start := strings.Index(s, "${")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end < 0 {
			break
		}
		name := s[start+2 : start+end]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		s = s[start+end+1:]
	}
	return names
}

func parseDataJSON(dir string) string {
	candidates := []string{"data.json"}
	for _, name := range candidates {
		envPath := filepath.Join(dir, name)
		data, err := os.ReadFile(envPath)
		if err != nil {
			continue
		}
		var appData struct {
			Image    string `json:"image"`
			Versions []struct {
				Name  string `json:"name"`
				Image string `json:"image"`
			} `json:"versions"`
		}
		if err := json.Unmarshal(data, &appData); err == nil {
			if appData.Image != "" {
				return appData.Image
			}
			for _, v := range appData.Versions {
				if v.Image != "" {
					return v.Image
				}
			}
		}
	}
	return ""
}

func scanAllEnvFiles(dir string) map[string]string {
	envMap := make(map[string]string)
	skipFiles := map[string]bool{
		"docker-compose.yml": true, "docker-compose.yaml": true,
		"readme.md": true, "readme.txt": true, "data.json": true,
	}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := strings.ToLower(info.Name())
		if skipFiles[name] {
			return nil
		}
		if info.Size() > 1024*1024 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		if !strings.Contains(content, "=") {
			return nil
		}
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "{") || strings.HasPrefix(line, "}") {
				continue
			}
			idx := strings.Index(line, "=")
			if idx < 0 || idx == 0 {
				continue
			}
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, `"'`)
			if key != "" && val != "" && !strings.Contains(key, " ") && len(key) < 64 {
				envMap[key] = val
			}
		}
		return nil
	})
	return envMap
}

func extractContainerPort(ports []string) int {
	for _, p := range ports {
		p = strings.TrimSpace(p)
		idx := strings.LastIndex(p, ":")
		if idx < 0 {
			if port, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				return port
			}
			continue
		}
		right := strings.TrimSpace(p[idx+1:])
		right = strings.Trim(right, "\"'")
		if port, err := strconv.Atoi(right); err == nil {
			return port
		}
	}
	return 0
}

func extractImageFromScripts(tarGzPath string) string {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return ""
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		name := hdr.Name
		base := filepath.Base(name)
		if base != "init.sh" && base != "upgrade.sh" {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			continue
		}
		content := string(data)
		re := regexp.MustCompile(`(?m)^([A-Z_]+_IMAGE)=(.+)`)
		matches := re.FindStringSubmatch(content)
		if len(matches) >= 3 {
			val := strings.TrimSpace(matches[2])
			val = strings.Trim(val, "\"'")
			if val != "" && !strings.Contains(val, "$") {
				return val
			}
		}
	}
	return ""
}

// downloadAppPackage 下载 1Panel 版本包并解压到目标目录
func downloadAppPackage(url, destDir string) error {
	global.LOG.Infof("[Install] downloading package from %s", url)
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
	tmpPath := filepath.Join(destDir, "app.pkg")
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return fmt.Errorf("save: %w", err)
	}
	defer os.Remove(tmpPath)

	if isZip(tmpPath) {
		if err := extractZip(tmpPath, destDir); err == nil {
			flattenSingleSubdir(destDir)
			global.LOG.Infof("[Install] extracted zip package")
			return nil
		}
		global.LOG.Warnf("[Install] zip extract failed: %v", err)
	} else if isTarGz(tmpPath) {
		if err := extractTarGz(tmpPath, destDir); err == nil {
			flattenSingleSubdir(destDir)
			global.LOG.Infof("[Install] extracted tar.gz package")
			return nil
		}
		global.LOG.Warnf("[Install] tar.gz extract failed: %v", err)
	} else {
		global.LOG.Warnf("[Install] unknown package format")
	}

	return fmt.Errorf("unsupported package format")
}

func isZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	header := make([]byte, 4)
	_, err = f.Read(header)
	return err == nil && header[0] == 0x50 && header[1] == 0x4B && header[2] == 0x03 && header[3] == 0x04
}

func isTarGz(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	header := make([]byte, 2)
	_, err = f.Read(header)
	return err == nil && header[0] == 0x1F && header[1] == 0x8B
}

func extractZip(src, dest string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, file := range zr.File {
		fpath, ok := safeJoin(dest, file.Name)
		if !ok {
			global.LOG.Warnf("[Install] zip path traversal blocked: %s", file.Name)
			continue
		}
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

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fpath, ok := safeJoin(dest, hdr.Name)
		if !ok {
			global.LOG.Warnf("[Install] tar path traversal blocked: %s", hdr.Name)
			continue
		}
		if hdr.Typeflag == tar.TypeDir {
			os.MkdirAll(fpath, os.FileMode(hdr.Mode))
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), 0755)
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			continue
		}
		_, err = io.Copy(outFile, tr)
		outFile.Close()
		if err != nil {
			continue
		}
	}
	return nil
}

func safeJoin(base, sub string) (string, bool) {
	joined := filepath.Join(base, filepath.Clean("/"+sub))
	absBase, _ := filepath.Abs(base)
	absJoined, _ := filepath.Abs(joined)
	if absBase != "" && !strings.HasPrefix(absJoined, absBase+string(os.PathSeparator)) && absJoined != absBase {
		return "", false
	}
	return joined, true
}

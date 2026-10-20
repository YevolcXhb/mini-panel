package service

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	"github.com/minipanel/minipanel/internal/utils/proxy"
	"gorm.io/gorm"
)

type SettingService struct {
	repo *repository.SettingRepository
}

func NewSettingService() *SettingService {
	return &SettingService{
		repo: repository.NewSettingRepository(global.DB),
	}
}

func (s *SettingService) Get(key string) (string, error) {
	item, err := s.repo.Get(key)
	if err != nil {
		return "", err
	}
	return item.Value, nil
}

func (s *SettingService) Set(key, value string) error {
	return s.repo.Set(key, value)
}

func (s *SettingService) List() (map[string]string, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, item := range items {
		// 代理密码不回传明文，前端展示为占位
		if item.Key == "proxy_config" {
			if c, perr := proxy.ParseJSON(item.Value); perr == nil && c.Password != "" {
				sanitized := *c
				sanitized.Password = "******"
				if b, jerr := json.Marshal(&sanitized); jerr == nil {
					item.Value = string(b)
				}
			}
		}
		result[item.Key] = item.Value
	}
	return result, nil
}

// UpdateProxyConfig 保存代理配置并立即热加载生效。
// 任一步骤失败（非法配置 / 连通性测试失败）都不改动现有出站行为。
func (s *SettingService) UpdateProxyConfig(raw string) error {
	c, err := proxy.ParseJSON(raw)
	if err != nil {
		return err
	}
	// 密码占位：前端未改密码时传回 ******，保留旧密码
	if c.Password == "******" {
		old, _ := s.Get("proxy_config")
		if oc, oerr := proxy.ParseJSON(old); oerr == nil {
			c.Password = oc.Password
		}
	}
	if err := proxy.Default.Apply(c); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.repo.Set("proxy_config", string(b))
}

// InitProxyConfig 启动时从数据库加载代理配置（不探测，直接应用，保证重启后代理仍然生效）
func (s *SettingService) InitProxyConfig() {
	raw, err := s.Get("proxy_config")
	if err != nil || raw == "" {
		return
	}
	c, err := proxy.ParseJSON(raw)
	if err != nil || !c.Enabled {
		return
	}
	// 启动时直接应用，不做连通性阻断——网络未就绪时直连也能工作，代理下个周期仍会生效
	if err := proxy.Default.Apply(c); err != nil {
		// 探测失败也退化为应用直连之外的配置？这里保守起见记录后放弃
		return
	}
}

func (s *SettingService) ClearData() error {
	models := []interface{}{
		&model.User{}, &model.App{}, &model.AppDetail{}, &model.AppInstall{},
		&model.AppSource{}, &model.Cronjob{}, &model.Setting{}, &model.LoginAttempt{}, &model.AuditLog{},
	}
	for _, m := range models {
		if err := global.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			return err
		}
	}
	logFile := filepath.Join(global.GetDataDir(), "logs", "panel.log")
	_ = os.Remove(logFile)
	if err := s.InitDefaults(); err != nil {
		return err
	}
	auth := NewAuthService()
	return auth.InitAdmin("admin", "admin123")
}

func (s *SettingService) InitDefaults() error {
	defaults := map[string]string{
		"theme":             "dark",
		"language":          "zh",
		"timezone":          "Asia/Shanghai",
		"container_mode":    "dockroot",
		"file_manager_root": "/",
		"SecurityEntrance":  "",
		"load_host_mode":    "chroot",
	}
	for k, v := range defaults {
		if _, err := s.repo.Get(k); err != nil {
			if err := s.repo.Set(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

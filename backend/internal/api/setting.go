package api

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

var allowedSettingKeys = map[string]bool{
	"theme":             true,
	"language":          true,
	"timezone":          true,
	"container_mode":    true,
	"file_manager_root": true,
	"SecurityEntrance":  true,
	"BindDomain":        true,
	"AllowIPs":          true,
	"load_host_mode":    true,
}

var entranceRe = regexp.MustCompile(`^/[a-zA-Z0-9_\-]+$`)
var domainSettingRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`)

func validateSettingValue(key, value string) error {
	if len(value) > 4096 {
		return fmt.Errorf("设置值过长")
	}
	switch key {
	case "SecurityEntrance":
		if value != "" && !entranceRe.MatchString(value) {
			return fmt.Errorf("安全入口格式非法（必须以 / 开头且仅含字母数字_-）")
		}
	case "BindDomain":
		if value != "" && !domainSettingRe.MatchString(strings.ToLower(value)) {
			return fmt.Errorf("绑定域名格式非法")
		}
	case "AllowIPs":
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if strings.Contains(item, "/") {
				if _, _, err := net.ParseCIDR(item); err != nil {
					return fmt.Errorf("AllowIPs 包含非法 CIDR: %s", item)
				}
			} else if net.ParseIP(item) == nil {
				return fmt.Errorf("AllowIPs 包含非法 IP: %s", item)
			}
		}
	case "file_manager_root":
		if value != "" && (!filepath.IsAbs(value) || strings.Contains(value, "..")) {
			return fmt.Errorf("文件管理根目录必须是绝对路径且不能包含 ..")
		}
	}
	return nil
}

type SettingAPI struct {
	service *service.SettingService
}

func NewSettingAPI() *SettingAPI {
	return &SettingAPI{service: service.NewSettingService()}
}

func (a *SettingAPI) Get(c *gin.Context) {
	items, err := a.service.List()
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: items})
}

func (a *SettingAPI) Update(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	for k, v := range req {
		if !allowedSettingKeys[k] {
			c.JSON(http.StatusOK, dto.Response{Code: 400, Message: "不允许修改的设置项: " + k})
			return
		}
		if err := validateSettingValue(k, v); err != nil {
			c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
			return
		}
		if err := a.service.Set(k, v); err != nil {
			c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "updated"})
}

func (a *SettingAPI) Reset(c *gin.Context) {
	if err := a.service.InitDefaults(); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "reset"})
}

func (a *SettingAPI) ClearData(c *gin.Context) {
	if err := a.service.ClearData(); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "all data cleared, please re-login with admin/admin123"})
}

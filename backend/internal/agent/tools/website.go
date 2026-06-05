package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/service"
)

// WebsiteListTool 列出网站
type WebsiteListTool struct{}

func (t *WebsiteListTool) Name() string                     { return "list_websites" }
func (t *WebsiteListTool) Description() string              { return "列出所有网站配置及状态。" }
func (t *WebsiteListTool) Parameters() []provider.ToolParam { return nil }
func (t *WebsiteListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	svc := service.NewWebsiteService()
	websites, err := svc.List()
	if err != nil {
		return "", err
	}
	if len(websites) == 0 {
		return "没有网站", nil
	}
	var sb strings.Builder
	for _, w := range websites {
		status := "已启用"
		if !w.Enabled {
			status = "已禁用"
		}
		ssl := "未启用"
		if w.SSL {
			ssl = "已启用"
		}
		sb.WriteString(fmt.Sprintf("ID: %d | 域名: %s | 根目录: %s | 类型: %s | SSL: %s | 状态: %s\n",
			w.ID, w.Domain, w.Root, w.Type, ssl, status))
	}
	return sb.String(), nil
}

// WebsiteOpTool 网站操作
type WebsiteOpTool struct{}

func (t *WebsiteOpTool) Name() string { return "website_op" }
func (t *WebsiteOpTool) Description() string {
	return "对网站执行操作: enable(启用)/disable(禁用)/reload_nginx/delete。"
}
func (t *WebsiteOpTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "id", Type: "integer", Description: "网站 ID", Required: true},
		{Name: "action", Type: "string", Description: "操作: enable/disable/reload_nginx/delete", Required: true},
	}
}
func (t *WebsiteOpTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	svc := service.NewWebsiteService()
	id := uint(GetInt(args, "id"))
	action := GetString(args, "action")
	switch action {
	case "enable":
		return "", svc.ToggleEnable(id, true)
	case "disable":
		return "", svc.ToggleEnable(id, false)
	case "reload_nginx":
		return "", svc.ReloadNginx()
	case "delete":
		return "", svc.Delete(id)
	default:
		return "", fmt.Errorf("不支持的操作: %s", action)
	}
}

// NginxLogTool 读取 Nginx 日志
type NginxLogTool struct{}

func (t *NginxLogTool) Name() string        { return "read_nginx_log" }
func (t *NginxLogTool) Description() string { return "读取 Nginx 访问日志或错误日志。" }
func (t *NginxLogTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "type", Type: "string", Description: "日志类型: access/error", Required: true},
		{Name: "tail", Type: "integer", Description: "行数，默认 30", Required: false},
	}
}
func (t *NginxLogTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	logType := GetString(args, "type")
	tail := GetInt(args, "tail")
	if tail <= 0 {
		tail = 30
	}
	var logPath string
	if logType == "error" {
		logPath = "/var/log/nginx/error.log"
	} else {
		logPath = "/var/log/nginx/access.log"
	}
	fsvc := service.NewFileService()
	contentBytes, err := fsvc.GetContent(logPath)
	if err != nil {
		return "", err
	}
	content := string(contentBytes)
	lines := strings.Split(content, "\n")
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
		content = strings.Join(lines, "\n")
	}
	return fmt.Sprintf("Nginx %s 日志 (最近 %d 行):\n%s", logType, len(lines), content), nil
}

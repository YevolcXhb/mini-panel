package agent

import (
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/tools"
)

// BuildToolRegistry 构建完整工具注册表
func BuildToolRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(&tools.SystemInfoTool{})
	r.Register(&tools.ProcessListTool{})
	r.Register(&tools.ContainerListTool{})
	r.Register(&tools.ContainerOpTool{})
	r.Register(&tools.ContainerLogsTool{})
	r.Register(&tools.WebsiteListTool{})
	r.Register(&tools.WebsiteOpTool{})
	r.Register(&tools.NginxLogTool{})
	r.Register(&tools.DatabaseListTool{})
	r.Register(&tools.DatabaseOpTool{})
	r.Register(&tools.FirewallListTool{})
	r.Register(&tools.FirewallOpTool{})
	r.Register(&tools.BackupListTool{})
	r.Register(&tools.BackupOpTool{})
	r.Register(&tools.FileReadTool{})
	r.Register(&tools.FileListTool{})
	r.Register(&tools.FileWriteTool{})
	r.Register(&tools.ExecTool{})
	r.Register(&tools.DashboardTool{})
	return r
}

// NewEngineWithProvider 使用 Provider 创建完整引擎
func NewEngineWithProvider(p provider.Provider) *Engine {
	registry := BuildToolRegistry()
	return NewEngine(p, registry, 20)
}

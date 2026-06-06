package skills

import (
	"github.com/minipanel/minipanel/internal/agent/tools"
)

// LoadBuiltinSkills 加载所有内置技能
func LoadBuiltinSkills() *Manager {
	mgr := NewManager()

	// 系统监控技能
	mgr.Register(&Skill{
		ID:          "system",
		Name:        "系统监控",
		Description: "查看系统信息、CPU、内存、磁盘、进程状态",
		Icon:        "Monitor",
		Tools: []tools.Tool{
			tools.NewSystemInfoTool(),
			tools.NewProcessListTool(),
		},
	})

	// 容器管理技能
	mgr.Register(&Skill{
		ID:          "container",
		Name:        "容器管理",
		Description: "管理 Docker 容器，包括列出、启动、停止、查看日志等",
		Icon:        "Box",
		Tools: []tools.Tool{
			tools.NewContainerListTool(),
			tools.NewContainerOpTool(),
			tools.NewContainerLogsTool(),
		},
	})

	// 网站管理技能
	mgr.Register(&Skill{
		ID:          "website",
		Name:        "网站管理",
		Description: "管理网站、Nginx 配置、SSL 证书等",
		Icon:        "Globe",
		Tools: []tools.Tool{
			tools.NewWebsiteListTool(),
			tools.NewWebsiteOpTool(),
			tools.NewNginxLogTool(),
		},
	})

	// 数据库管理技能
	mgr.Register(&Skill{
		ID:          "database",
		Name:        "数据库管理",
		Description: "管理 MySQL、Redis、PostgreSQL 等数据库",
		Icon:        "Coin",
		Tools: []tools.Tool{
			tools.NewDatabaseListTool(),
			tools.NewDatabaseOpTool(),
		},
	})

	// 防火墙技能
	mgr.Register(&Skill{
		ID:          "firewall",
		Name:        "防火墙管理",
		Description: "管理防火墙规则、端口开放、IP 黑白名单",
		Icon:        "Lock",
		Tools: []tools.Tool{
			tools.NewFirewallListTool(),
			tools.NewFirewallOpTool(),
		},
	})

	// 文件管理技能
	mgr.Register(&Skill{
		ID:          "file",
		Name:        "文件管理",
		Description: "读取、编辑、创建、删除服务器文件",
		Icon:        "Document",
		Tools: []tools.Tool{
			tools.NewFileReadTool(),
			tools.NewFileListTool(),
			tools.NewFileWriteTool(),
		},
	})

	// 备份恢复技能
	mgr.Register(&Skill{
		ID:          "backup",
		Name:        "备份恢复",
		Description: "管理备份任务、执行备份、恢复数据",
		Icon:        "FolderChecked",
		Tools: []tools.Tool{
			tools.NewBackupListTool(),
			tools.NewBackupOpTool(),
		},
	})

	// 网络搜索技能
	mgr.Register(&Skill{
		ID:           "web",
		Name:         "网络搜索",
		Description:  "网页搜索、网页内容抓取",
		Icon:         "Search",
		SystemPrompt: "当用户询问需要实时网络信息的问题时，你可以使用 web_search 工具搜索最新信息，或使用 web_fetch 工具抓取特定网页内容。",
		Tools: []tools.Tool{
			tools.NewWebSearchTool(),
			tools.NewWebFetchTool(),
		},
	})

	return mgr
}

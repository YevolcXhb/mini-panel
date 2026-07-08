package service

// Feature 描述面板中的一个功能模块（菜单项/路由）
type Feature struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

// Features 是面板全部功能模块的静态注册表，key 与前端路由 path 对齐
var Features = []Feature{
	// 概览
	{Key: "/dashboard", Name: "仪表盘", Group: "概览"},
	{Key: "/monitor", Name: "监控中心", Group: "概览"},
	// 数据
	{Key: "/backups", Name: "备份恢复", Group: "数据"},
	{Key: "/databases", Name: "数据库", Group: "数据"},
	{Key: "/files", Name: "文件管理", Group: "数据"},
	// 服务
	{Key: "/containers", Name: "容器管理", Group: "服务"},
	{Key: "/apps", Name: "应用商店", Group: "服务"},
	{Key: "/websites", Name: "网站管理", Group: "服务"},
	{Key: "/firewall", Name: "防火墙", Group: "服务"},
	{Key: "/processes", Name: "进程管理", Group: "服务"},
	{Key: "/cronjobs", Name: "计划任务", Group: "服务"},
	// 运维
	{Key: "/ssh", Name: "SSH 管理", Group: "运维"},
	{Key: "/agent", Name: "Mini Agent", Group: "运维"},
	{Key: "/logs", Name: "系统日志", Group: "运维"},
	// 系统
	{Key: "/settings", Name: "系统设置", Group: "系统"},
	// 用户管理仅管理员可见，不在此处开放
}

// AdminFeatures 是管理员默认拥有的全部权限 key 集合
func AdminFeatures() []string {
	keys := make([]string, 0, len(Features))
	for _, f := range Features {
		keys = append(keys, f.Key)
	}
	return keys
}

// UserDefaultFeatures 普通用户默认拥有的最小权限集合
func UserDefaultFeatures() []string {
	return []string{"/dashboard", "/monitor", "/logs"}
}

// HasFeature 检查用户是否拥有某项功能权限
// 管理员始终放行；普通用户按 permissions 列表判断
func HasFeature(role string, permissions []string, featureKey string) bool {
	if role == "admin" {
		return true
	}
	if permissions == nil {
		// 兼容老数据：未显式配置的普通用户使用默认最小集
		for _, k := range UserDefaultFeatures() {
			if k == featureKey {
				return true
			}
		}
		return false
	}
	for _, k := range permissions {
		if k == featureKey {
			return true
		}
	}
	return false
}

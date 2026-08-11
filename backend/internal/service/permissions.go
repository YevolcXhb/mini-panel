package service

import "github.com/minipanel/minipanel/internal/permission"

// Features 是面板全部功能模块的静态注册表（定义见 internal/permission）
var Features = permission.Features

// AdminFeatures 是管理员默认拥有的全部权限 key 集合
func AdminFeatures() []string {
	return permission.AdminFeatures()
}

// UserDefaultFeatures 普通用户默认拥有的最小权限集合
func UserDefaultFeatures() []string {
	return permission.UserDefaultFeatures()
}

// HasFeature 检查用户是否拥有某项功能权限
// 管理员始终放行；普通用户按 permissions 列表判断
func HasFeature(role string, permissions []string, featureKey string) bool {
	return permission.HasFeature(role, permissions, featureKey)
}

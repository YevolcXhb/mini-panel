package agent

import (
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/skills"
	"github.com/minipanel/minipanel/internal/agent/tools"
)

var skillManager = skills.LoadBuiltinSkills()

// BuildToolRegistryBySkills 根据启用的技能构建工具注册表
func BuildToolRegistryBySkills(skillIDs []string) *tools.Registry {
	if len(skillIDs) == 0 {
		// 默认启用所有技能
		skillIDs = []string{"system", "container", "website", "database", "firewall", "file", "backup", "web"}
	}
	return skillManager.BuildRegistry(skillIDs)
}

// GetAllSkills 获取所有内置技能列表
func GetAllSkills() []*skills.Skill {
	return skillManager.List()
}

// NewEngineWithProvider 使用 Provider 创建完整引擎
func NewEngineWithProvider(p provider.Provider, skillIDs []string, systemPrompt string) *Engine {
	registry := BuildToolRegistryBySkills(skillIDs)
	return NewEngine(p, registry, systemPrompt, 30)
}

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
	registry := skillManager.BuildRegistry(skillIDs)
	// 核心工具：始终注册，不受技能开关影响
	registry.Register(tools.NewResolveLazyRefTool())
	return registry
}

// GetAllSkills 获取所有内置技能列表
func GetAllSkills() []*skills.Skill {
	return skillManager.List()
}

// NewEngineWithProvider 使用 Provider 创建完整引擎
func NewEngineWithProvider(p provider.Provider, skillIDs []string, systemPrompt string) *Engine {
	registry := BuildToolRegistryBySkills(skillIDs)
	return NewEngine(p, registry, systemPrompt, 10)
}

// NewOrchestratorWithProvider 创建带三阶段编排的引擎
func NewOrchestratorWithProvider(p provider.Provider, skillIDs []string, systemPrompt string) (*Engine, *Orchestrator) {
	registry := BuildToolRegistryBySkills(skillIDs)
	engine := NewEngine(p, registry, systemPrompt, 10)
	orchestrator := NewOrchestrator(engine)
	return engine, orchestrator
}

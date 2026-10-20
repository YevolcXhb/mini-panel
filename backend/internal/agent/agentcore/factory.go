package agentcore

import (
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/skills"
	"github.com/minipanel/minipanel/internal/agent/tools"
	"github.com/minipanel/minipanel/internal/model"
)

// skillManager 内置技能注册表（与旧 agent 包共享同一份内置技能定义）。
var skillManager = skills.LoadBuiltinSkills()

// BuildToolRegistryBySkills 按启用的技能构建工具注册表。
func BuildToolRegistryBySkills(skillIDs []string) *tools.Registry {
	if len(skillIDs) == 0 {
		skillIDs = []string{"system", "container", "website", "database", "firewall", "file", "backup", "web"}
	}
	registry := skillManager.BuildRegistry(skillIDs)
	// 核心工具：始终注册，不受技能开关影响
	registry.Register(tools.NewResolveLazyRefTool())
	return registry
}

// GetAllSkills 获取所有内置技能。
func GetAllSkills() []*skills.Skill {
	return skillManager.List()
}

// NewFromConfig 用数据库里的 AgentConfig 构造引擎。
//
// 相比旧工厂多做的三件事：
//   - 把 ContextWindow / MaxTokens / MaxSteps 传进引擎，让上下文预算可配置
//   - Provider 走注册表，未知 provider 直接报错而不是静默降级
//   - 系统提示词为空时回落到内置运维提示词，避免模型无约束乱调工具
func NewFromConfig(cfg *model.AgentConfig, skillIDs []string) (*Engine, error) {
	p, err := provider.NewProvider(
		cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model,
		float32(cfg.Temperature), cfg.MaxTokens,
	)
	if err != nil {
		return nil, err
	}

	sysPrompt := cfg.SystemPrompt
	registry := BuildToolRegistryBySkills(skillIDs)

	return NewEngine(p, registry, Options{
		SystemPrompt:    sysPrompt,
		MaxSteps:        cfg.MaxSteps,
		MaxErrors:       5,
		ContextWindow:   cfg.ContextWindow,
		MaxOutputTokens: cfg.MaxTokens,
	}), nil
}

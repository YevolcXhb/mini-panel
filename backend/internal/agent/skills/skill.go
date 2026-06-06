package skills

import (
	"github.com/minipanel/minipanel/internal/agent/tools"
)

// Skill 技能插件
type Skill struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Icon         string       `json:"icon"`
	Tools        []tools.Tool `json:"-"`
	SystemPrompt string       `json:"system_prompt"`
}

// Manager 技能管理器
type Manager struct {
	skills map[string]*Skill
}

func NewManager() *Manager {
	return &Manager{skills: make(map[string]*Skill)}
}

func (m *Manager) Register(skill *Skill) {
	m.skills[skill.ID] = skill
}

func (m *Manager) Get(id string) (*Skill, bool) {
	s, ok := m.skills[id]
	return s, ok
}

func (m *Manager) List() []*Skill {
	var list []*Skill
	for _, s := range m.skills {
		list = append(list, s)
	}
	return list
}

// BuildRegistry 根据启用的技能列表构建工具注册表
func (m *Manager) BuildRegistry(skillIDs []string) *tools.Registry {
	registry := tools.NewRegistry()
	for _, id := range skillIDs {
		if skill, ok := m.skills[id]; ok {
			for _, t := range skill.Tools {
				registry.Register(t)
			}
		}
	}
	return registry
}

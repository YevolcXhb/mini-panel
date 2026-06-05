package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// Tool 是所有工具的接口
type Tool interface {
	Name() string
	Description() string
	Parameters() []provider.ToolParam
	Execute(ctx context.Context, args map[string]interface{}) (string, error)
}

// Registry 工具注册表
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	var list []Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// ToDefinitions 转换为 LLM 可用的工具定义
func (r *Registry) ToDefinitions() []provider.ToolDefinition {
	var defs []provider.ToolDefinition
	for _, t := range r.tools {
		params := map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		}
		properties := params["properties"].(map[string]interface{})
		required := params["required"].([]string)
		for _, p := range t.Parameters() {
			properties[p.Name] = map[string]string{
				"type":        p.Type,
				"description": p.Description,
			}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		params["required"] = required

		defs = append(defs, provider.ToolDefinition{
			Type: "function",
			Function: struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Parameters  map[string]interface{} `json:"parameters"`
			}{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  params,
			},
		})
	}
	return defs
}

// Executor 工具执行器
type Executor struct {
	registry *Registry
}

func NewExecutor(registry *Registry) *Executor {
	return &Executor{registry: registry}
}

func (e *Executor) Execute(ctx context.Context, call provider.ToolCall) (string, error) {
	tool, ok := e.registry.Get(call.Function.Name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", call.Function.Name)
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("parse tool arguments: %w", err)
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil // 返回错误但不中断流程
	}
	return result, nil
}

// --- 通用辅助函数 ---

// GetString 从 args 安全获取 string
func GetString(args map[string]interface{}, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetInt 从 args 安全获取 int
func GetInt(args map[string]interface{}, key string) int {
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	default:
		return 0
	}
}

// GetBool 从 args 安全获取 bool
func GetBool(args map[string]interface{}, key string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.ToLower(val) == "true"
	default:
		return false
	}
}

// FormatJSON 格式化对象为 JSON 字符串
func FormatJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

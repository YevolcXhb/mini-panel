package provider

import (
	"context"
	"encoding/json"
)

// LLMMessage 统一消息格式
type LLMMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall 工具调用
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolParam 工具参数定义
type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolDefinition 工具定义（传给 LLM）
type ToolDefinition struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

// LLMResponse LLM 响应
type LLMResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
	Usage     struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Provider LLM Provider 接口
type Provider interface {
	Chat(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (*LLMResponse, error)
	SupportsToolCalling() bool
}

// NewProvider 根据配置创建 Provider
func NewProvider(providerType, baseURL, apiKey, model string, temperature float32, maxTokens int) (Provider, error) {
	switch providerType {
	case "anthropic":
		return NewAnthropicProvider(baseURL, apiKey, model, temperature, maxTokens), nil
	case "ollama":
		return NewOpenAIProvider(baseURL, apiKey, model, temperature, maxTokens), nil
	default:
		return NewOpenAIProvider(baseURL, apiKey, model, temperature, maxTokens), nil
	}
}

// BuildToolDefinitions 将工具列表转为 LLM 可用的定义
func BuildToolDefinitions(tools []interface { /* 具体类型在 tools 包定义 */
}) []ToolDefinition {
	// 由 tools 包调用
	return nil
}

// MustMarshalJSON 辅助函数
func MustMarshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

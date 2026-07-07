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

// StreamDelta 流式响应的单个分片。
// Type 取值：
//   - "content"   文本内容增量（Content 字段填充）
//   - "tool_call" 工具调用（流结束前一次性发送完整调用，ToolCallIndex/ToolCallID/FunctionName/ArgumentsDelta 填充）
//   - "usage"     用量统计（Usage 填充，通常在流结束时发送一次）
//   - "done"      流结束（无数据，仅信号）
//   - "error"     流内部错误（Content 字段填充错误信息）
type StreamDelta struct {
	Type           string `json:"type"`
	Content        string `json:"content,omitempty"`
	ToolCallIndex  int    `json:"tool_call_index,omitempty"`
	ToolCallID     string `json:"tool_call_id,omitempty"`
	ToolCallType   string `json:"tool_call_type,omitempty"`
	FunctionName   string `json:"function_name,omitempty"`
	ArgumentsDelta string `json:"arguments_delta,omitempty"`
	Usage          *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// Provider LLM Provider 接口
type Provider interface {
	Chat(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (*LLMResponse, error)
	SupportsToolCalling() bool
}

// Streamer 流式调用接口（可选实现）。
// Provider 若实现此接口，Engine 会优先走流式路径以实现真正的逐 token 推送。
type Streamer interface {
	ChatStream(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (<-chan StreamDelta, error)
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

// MustMarshalJSON 辅助函数
func MustMarshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

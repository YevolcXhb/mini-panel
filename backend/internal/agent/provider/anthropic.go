package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider Claude API
type AnthropicProvider struct {
	client      *http.Client
	baseURL     string
	apiKey      string
	model       string
	temperature float32
	maxTokens   int
}

func NewAnthropicProvider(baseURL, apiKey, model string, temperature float32, maxTokens int) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &AnthropicProvider{
		client:      &http.Client{Timeout: 120 * time.Second},
		baseURL:     baseURL,
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
	}
}

func (p *AnthropicProvider) SupportsToolCalling() bool { return true }

func (p *AnthropicProvider) Chat(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (*LLMResponse, error) {
	// Claude 格式转换: system 消息需要单独提取
	var systemPrompt string
	var claudeMessages []map[string]interface{}
	for _, m := range messages {
		if m.Role == "system" {
			if systemPrompt != "" {
				systemPrompt += "\n"
			}
			systemPrompt += m.Content
			continue
		}
		msg := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.Role == "tool" {
			msg["role"] = "user"
			msg["content"] = fmt.Sprintf("<tool_result>\nTool: %s\nResult: %s\n</tool_result>", m.ToolCallID, m.Content)
		}
		claudeMessages = append(claudeMessages, msg)
	}

	reqBody := map[string]interface{}{
		"model":       p.model,
		"max_tokens":  p.maxTokens,
		"messages":    claudeMessages,
		"temperature": p.temperature,
	}
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}
	if len(tools) > 0 {
		var claudeTools []map[string]interface{}
		for _, t := range tools {
			claudeTools = append(claudeTools, map[string]interface{}{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": t.Function.Parameters,
			})
		}
		reqBody["tools"] = claudeTools
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			ID    string          `json:"id"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	llmResp := &LLMResponse{}
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			llmResp.Content += c.Text
		case "tool_use":
			args, _ := json.Marshal(c.Input)
			llmResp.ToolCalls = append(llmResp.ToolCalls, ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      c.Name,
					Arguments: string(args),
				},
			})
		}
	}
	llmResp.Usage.PromptTokens = result.Usage.InputTokens
	llmResp.Usage.CompletionTokens = result.Usage.OutputTokens
	llmResp.Usage.TotalTokens = result.Usage.InputTokens + result.Usage.OutputTokens
	return llmResp, nil
}

package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		client:      defaultHTTPClient,
		baseURL:     baseURL,
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
	}
}

func (p *AnthropicProvider) SupportsToolCalling() bool { return true }

// ChatStream 流式调用 Anthropic Messages API（SSE）。
// Anthropic SSE 事件类型：message_start / content_block_start / content_block_delta / content_block_stop / message_delta / message_stop
func (p *AnthropicProvider) ChatStream(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (<-chan StreamDelta, error) {
	systemPrompt, claudeMessages := claudeMessagesFromLLM(messages)

	reqBody := map[string]interface{}{
		"model":       p.model,
		"max_tokens":  p.maxTokens,
		"messages":    claudeMessages,
		"temperature": p.temperature,
		"stream":      true,
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
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Transport: defaultHTTPClient.Transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic stream error %d: %s", resp.StatusCode, string(body))
	}

	out := make(chan StreamDelta, 64)
	go func() {
		defer func() {
			resp.Body.Close()
			close(out)
		}()

		// 工具调用累积（content_block_index → accum）
		type toolAccum struct {
			ID        string
			Name      string
			Arguments string
		}
		toolMap := map[int]*toolAccum{}

		decoder := bufio.NewReader(resp.Body)
		var currentBlockIdx int

		for {
			select {
			case <-ctx.Done():
				out <- StreamDelta{Type: "done"}
				return
			default:
			}

			line, err := decoder.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					out <- StreamDelta{Type: "done"}
					return
				}
				out <- StreamDelta{Type: "error", Content: err.Error()}
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Anthropic SSE 格式：event: xxx\ndata: {...}
			if strings.HasPrefix(line, "event:") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			var event struct {
				Type         string `json:"type"`
				Index        int    `json:"index"`
				ContentBlock *struct {
					Type  string          `json:"type"`
					Text  string          `json:"text"`
					ID    string          `json:"id"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content_block"`
				Delta *struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
				Message *struct {
					Usage struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				} `json:"message"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "content_block_start":
				currentBlockIdx = event.Index
				if event.ContentBlock != nil {
					switch event.ContentBlock.Type {
					case "text":
						// 文本块开始，无需操作（等待 delta）
					case "tool_use":
						toolMap[event.Index] = &toolAccum{
							ID:   event.ContentBlock.ID,
							Name: event.ContentBlock.Name,
						}
					}
				}
			case "content_block_delta":
				if event.Delta != nil {
					switch event.Delta.Type {
					case "text_delta":
						out <- StreamDelta{Type: "content", Content: event.Delta.Text}
					case "input_json_delta":
						if accum, ok := toolMap[currentBlockIdx]; ok {
							accum.Arguments += event.Delta.PartialJSON
						}
					}
				}
			case "content_block_stop":
				// 工具调用块结束时发送完整工具调用
				if accum, ok := toolMap[event.Index]; ok {
					out <- StreamDelta{
						Type:           "tool_call",
						ToolCallIndex:  event.Index,
						ToolCallID:     accum.ID,
						ToolCallType:   "function",
						FunctionName:   accum.Name,
						ArgumentsDelta: accum.Arguments,
					}
				}
			case "message_delta":
				// 消息级别的 usage 增量（output_tokens）
			case "message_stop":
				if event.Usage != nil {
					out <- StreamDelta{
						Type: "usage",
						Usage: &struct {
							PromptTokens     int `json:"prompt_tokens"`
							CompletionTokens int `json:"completion_tokens"`
							TotalTokens      int `json:"total_tokens"`
						}{
							PromptTokens:     event.Usage.InputTokens,
							CompletionTokens: event.Usage.OutputTokens,
							TotalTokens:      event.Usage.InputTokens + event.Usage.OutputTokens,
						},
					}
				}
				out <- StreamDelta{Type: "done"}
				return
			}
		}
	}()

	return out, nil
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (*LLMResponse, error) {
	// Claude 格式转换: system 消息需要单独提取
	systemPrompt, claudeMessages := claudeMessagesFromLLM(messages)

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

// claudeMessagesFromLLM 把统一的 LLMMessage 列表转换为 Anthropic Messages API 格式。
// assistant 消息会携带 text + tool_use 内容块，tool 消息转换为带 tool_use_id 的
// tool_result 用户消息，保证多轮工具调用能被 Claude 正确关联。
func claudeMessagesFromLLM(messages []LLMMessage) (string, []map[string]interface{}) {
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
		if m.Role == "tool" {
			claudeMessages = append(claudeMessages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     m.Content,
					},
				},
			})
			continue
		}
		msg := map[string]interface{}{"role": m.Role}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			var blocks []map[string]interface{}
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var args interface{} = map[string]interface{}{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = map[string]interface{}{"raw": tc.Function.Arguments}
				}
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": args,
				})
			}
			msg["content"] = blocks
		} else {
			msg["content"] = m.Content
		}
		claudeMessages = append(claudeMessages, msg)
	}
	return systemPrompt, claudeMessages
}

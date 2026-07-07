package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var defaultHTTPClient = &http.Client{
	Timeout: 180 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// OpenAIProvider 支持 OpenAI / DeepSeek / Ollama 等兼容格式
type OpenAIProvider struct {
	client      *http.Client
	baseURL     string
	apiKey      string
	model       string
	temperature float32
	maxTokens   int
}

func NewOpenAIProvider(baseURL, apiKey, model string, temperature float32, maxTokens int) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &OpenAIProvider{
		client:      defaultHTTPClient,
		baseURL:     baseURL,
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
	}
}

func (p *OpenAIProvider) SupportsToolCalling() bool { return true }

// ChatStream 流式调用 OpenAI 兼容接口（SSE）。
// 返回的 channel 在流结束或出错时关闭；首个错误通过 errch 传出（若有）。
// 工具调用增量按 index 聚合，最终在流结束前发送完整 ToolCall。
func (p *OpenAIProvider) ChatStream(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (<-chan StreamDelta, error) {
	reqBody := map[string]interface{}{
		"model":       p.model,
		"messages":    messages,
		"temperature": p.temperature,
		"max_tokens":  p.maxTokens,
		"stream":      true,
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
		reqBody["tool_choice"] = "auto"
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	// 流式请求不能使用带 Timeout 的 defaultHTTPClient，新建无超时 client
	client := &http.Client{Transport: defaultHTTPClient.Transport}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("llm stream error %d: %s", resp.StatusCode, string(body))
	}

	out := make(chan StreamDelta, 64)
	go func() {
		defer func() {
			resp.Body.Close()
			close(out)
		}()

		// 累积工具调用（按 index 聚合）
		type toolAccum struct {
			ID        string
			Type      string
			Name      string
			Arguments string
		}
		toolMap := map[int]*toolAccum{}

		decoder := bufio.NewReader(resp.Body)
		for {
			select {
			case <-ctx.Done():
				out <- StreamDelta{Type: "done"}
				return
			default:
			}

			// SSE 每行以 "data: " 开头
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
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				out <- StreamDelta{Type: "done"}
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			// 文本增量
			if choice.Delta.Content != "" {
				out <- StreamDelta{Type: "content", Content: choice.Delta.Content}
			}

			// 工具调用增量（累积，流结束前一次性发送）
			for _, tc := range choice.Delta.ToolCalls {
				accum, exists := toolMap[tc.Index]
				if !exists {
					accum = &toolAccum{}
					toolMap[tc.Index] = accum
				}
				if tc.ID != "" {
					accum.ID = tc.ID
				}
				if tc.Type != "" {
					accum.Type = tc.Type
				}
				if tc.Function.Name != "" {
					accum.Name = tc.Function.Name
				}
				accum.Arguments += tc.Function.Arguments
			}

			// 流结束（finish_reason 非空）时，发送累积的工具调用和 usage
			if choice.FinishReason != "" {
				// 按 index 顺序发送工具调用
				maxIdx := -1
				for idx := range toolMap {
					if idx > maxIdx {
						maxIdx = idx
					}
				}
				for i := 0; i <= maxIdx; i++ {
					if accum, ok := toolMap[i]; ok {
						out <- StreamDelta{
							Type:           "tool_call",
							ToolCallIndex:  i,
							ToolCallID:     accum.ID,
							ToolCallType:   accum.Type,
							FunctionName:   accum.Name,
							ArgumentsDelta: accum.Arguments,
						}
					}
				}
				if chunk.Usage != nil {
					out <- StreamDelta{Type: "usage", Usage: chunk.Usage}
				}
				out <- StreamDelta{Type: "done"}
				return
			}
		}
	}()

	return out, nil
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []LLMMessage, tools []ToolDefinition) (*LLMResponse, error) {
	reqBody := map[string]interface{}{
		"model":       p.model,
		"messages":    messages,
		"temperature": p.temperature,
		"max_tokens":  p.maxTokens,
	}
	if len(tools) > 0 {
		reqBody["tools"] = tools
		reqBody["tool_choice"] = "auto"
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	msg := result.Choices[0].Message
	llmResp := &LLMResponse{
		Content: msg.Content,
		Usage:   result.Usage,
	}
	for _, tc := range msg.ToolCalls {
		llmResp.ToolCalls = append(llmResp.ToolCalls, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return llmResp, nil
}

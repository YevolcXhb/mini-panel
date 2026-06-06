package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/repository"
	"github.com/minipanel/minipanel/internal/agent/tools"
)

// Engine ReAct 引擎
type Engine struct {
	provider     provider.Provider
	registry     *tools.Registry
	executor     *tools.Executor
	sessionMgr   *repository.SessionManager
	systemPrompt string
	maxSteps     int
}

// NewEngine 创建引擎
func NewEngine(p provider.Provider, registry *tools.Registry, systemPrompt string, maxSteps int) *Engine {
	return &Engine{
		provider:     p,
		registry:     registry,
		executor:     tools.NewExecutor(registry),
		sessionMgr:   repository.NewSessionManager(),
		systemPrompt: systemPrompt,
		maxSteps:     maxSteps,
	}
}

// Run 执行一轮对话（ReAct 循环）
func (e *Engine) Run(ctx context.Context, sessionID uint, userInput string, stream chan<- StreamChunk) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("engine panic: %v", r)
			stream <- StreamChunk{Type: "error", Error: fmt.Sprintf("引擎内部错误: %v", r)}
			stream <- StreamChunk{Type: "done", Success: false}
		}
	}()

	messages, err := e.sessionMgr.LoadMessages(sessionID)
	if err != nil {
		stream <- StreamChunk{Type: "error", Error: "加载会话失败: " + err.Error()}
		return err
	}

	// 注入系统提示词
	if e.systemPrompt != "" {
		hasSystem := false
		for _, m := range messages {
			if m.Role == "system" {
				hasSystem = true
				break
			}
		}
		if !hasSystem {
			messages = append([]provider.LLMMessage{{Role: "system", Content: e.systemPrompt}}, messages...)
		}
	}

	messages = append(messages, provider.LLMMessage{Role: "user", Content: userInput})
	if err := e.sessionMgr.SaveUserMessage(sessionID, userInput); err != nil {
		stream <- StreamChunk{Type: "error", Error: "保存消息失败: " + err.Error()}
		return err
	}

	toolDefs := e.registry.ToDefinitions()
	for step := 0; step < e.maxSteps; step++ {
		resp, err := e.provider.Chat(ctx, messages, toolDefs)
		if err != nil {
			stream <- StreamChunk{Type: "error", Error: "LLM 调用失败: " + err.Error()}
			return err
		}

		if len(resp.ToolCalls) == 0 {
			content := resp.Content
			if strings.TrimSpace(content) == "" {
				content = "工具执行完成，结果已展示。"
			}
			stream <- StreamChunk{Type: "message", Content: content}
			messages = append(messages, provider.LLMMessage{Role: "assistant", Content: content})
			_ = e.sessionMgr.SaveAssistantMessage(sessionID, content, nil)
			stream <- StreamChunk{Type: "done", Success: true}
			return nil
		}

		messages = append(messages, provider.LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, resp.ToolCalls)

		var toolResults []provider.LLMMessage
		for _, tc := range resp.ToolCalls {
			stream <- StreamChunk{
				Type:       "tool_call",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    tc.Function.Arguments,
			}

			result, err := e.executor.Execute(ctx, tc)
			if err != nil {
				if confirmErr, ok := err.(*ConfirmRequiredError); ok {
					stream <- StreamChunk{
						Type:       "confirm_required",
						ToolCallID: tc.ID,
						Command:    confirmErr.Command,
						Message:    confirmErr.Message,
					}
					stream <- StreamChunk{Type: "done", Success: false, Error: "等待用户确认"}
					return nil
				}
				result = fmt.Sprintf("Error: %v", err)
			}

			stream <- StreamChunk{
				Type:       "tool_result",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    result,
			}

			toolResults = append(toolResults, provider.LLMMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			})
			_ = e.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, result)
		}

		messages = append(messages, toolResults...)
		messages = e.sessionMgr.CompressIfNeeded(messages)
	}

	stream <- StreamChunk{Type: "error", Error: "达到最大步数限制"}
	stream <- StreamChunk{Type: "done", Success: false}
	return fmt.Errorf("max steps exceeded")
}

// RunWithConfirm 带确认的流程继续执行
func (e *Engine) RunWithConfirm(ctx context.Context, sessionID uint, toolCallID string, confirmed bool, stream chan<- StreamChunk) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("engine panic: %v", r)
			stream <- StreamChunk{Type: "error", Error: fmt.Sprintf("引擎内部错误: %v", r)}
			stream <- StreamChunk{Type: "done", Success: false}
		}
	}()

	if !confirmed {
		result := "用户取消了此操作"
		_ = e.sessionMgr.SaveToolResult(sessionID, toolCallID, "", result)
		return e.continueAfterTool(ctx, sessionID, stream)
	}

	// 用户确认后，从会话历史中找到对应的 tool call 并执行
	messages, err := e.sessionMgr.LoadMessages(sessionID)
	if err != nil {
		stream <- StreamChunk{Type: "error", Error: "加载会话失败: " + err.Error()}
		return err
	}

	var targetTC *provider.ToolCall
	for _, m := range messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for i := range m.ToolCalls {
				if m.ToolCalls[i].ID == toolCallID {
					targetTC = &m.ToolCalls[i]
					break
				}
			}
		}
		if targetTC != nil {
			break
		}
	}

	if targetTC == nil {
		stream <- StreamChunk{Type: "error", Error: "找不到工具调用信息"}
		return fmt.Errorf("tool call not found: %s", toolCallID)
	}

	stream <- StreamChunk{
		Type:       "tool_call",
		ToolCallID: targetTC.ID,
		ToolName:   targetTC.Function.Name,
		Content:    targetTC.Function.Arguments,
	}

	result, err := e.executor.Execute(ctx, *targetTC)
	if err != nil {
		result = fmt.Sprintf("Error: %v", err)
	}

	stream <- StreamChunk{
		Type:       "tool_result",
		ToolCallID: targetTC.ID,
		ToolName:   targetTC.Function.Name,
		Content:    result,
	}

	_ = e.sessionMgr.SaveToolResult(sessionID, targetTC.ID, targetTC.Function.Name, result)
	return e.continueAfterTool(ctx, sessionID, stream)
}

func (e *Engine) continueAfterTool(ctx context.Context, sessionID uint, stream chan<- StreamChunk) error {
	messages, err := e.sessionMgr.LoadMessages(sessionID)
	if err != nil {
		stream <- StreamChunk{Type: "error", Error: err.Error()}
		return err
	}

	toolDefs := e.registry.ToDefinitions()
	resp, err := e.provider.Chat(ctx, messages, toolDefs)
	if err != nil {
		stream <- StreamChunk{Type: "error", Error: err.Error()}
		return err
	}

	if len(resp.ToolCalls) == 0 {
		stream <- StreamChunk{Type: "message", Content: resp.Content}
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
		stream <- StreamChunk{Type: "done", Success: true}
		return nil
	}

	stream <- StreamChunk{Type: "message", Content: resp.Content}
	_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, resp.ToolCalls)
	stream <- StreamChunk{Type: "done", Success: true}
	return nil
}

// IsDestructiveCommand 判断是否为破坏性命令
func IsDestructiveCommand(command string) bool {
	destructive := []string{"rm ", "kill ", "shutdown", "reboot", "poweroff", "systemctl stop", "systemctl restart",
		"docker rm", "docker stop", "docker restart", "pkill", "killall", "iptables -F", "ufw disable"}
	lower := strings.ToLower(command)
	for _, d := range destructive {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

// BuildTitleFromContent 根据内容生成会话标题
func BuildTitleFromContent(content string) string {
	if len(content) <= 20 {
		return content
	}
	return content[:20] + "..."
}

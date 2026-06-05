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
	provider   provider.Provider
	registry   *tools.Registry
	executor   *tools.Executor
	sessionMgr *repository.SessionManager
	maxSteps   int
}

// NewEngine 创建引擎
func NewEngine(p provider.Provider, registry *tools.Registry, maxSteps int) *Engine {
	return &Engine{
		provider:   p,
		registry:   registry,
		executor:   tools.NewExecutor(registry),
		sessionMgr: repository.NewSessionManager(),
		maxSteps:   maxSteps,
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
			stream <- StreamChunk{Type: "message", Content: resp.Content}
			messages = append(messages, provider.LLMMessage{Role: "assistant", Content: resp.Content})
			_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
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
	stream <- StreamChunk{Type: "message", Content: "已确认，请重新描述您的需求以继续。"}
	stream <- StreamChunk{Type: "done", Success: true}
	return nil
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

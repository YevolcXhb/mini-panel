package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/repository"
	"github.com/minipanel/minipanel/internal/agent/tools"
	"github.com/minipanel/minipanel/internal/global"
)

type StepState int

const (
	StepStateThinking StepState = iota
	StepStateCallingTool
	StepStateReflecting
	StepStateCompleted
)

type Engine struct {
	provider     provider.Provider
	registry     *tools.Registry
	executor     *tools.Executor
	sessionMgr   *repository.SessionManager
	systemPrompt string
	maxSteps     int
	maxErrors    int
}

func NewEngine(p provider.Provider, registry *tools.Registry, systemPrompt string, maxSteps int) *Engine {
	return &Engine{
		provider:     p,
		registry:     registry,
		executor:     tools.NewExecutor(registry),
		sessionMgr:   repository.NewSessionManager(),
		systemPrompt: systemPrompt,
		maxSteps:     maxSteps,
		maxErrors:    3,
	}
}

func (e *Engine) Run(ctx context.Context, sessionID uint, userInput string, stream chan<- StreamChunk) (err error) {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Engine] panic recovered: %v", r)
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

	messages = e.ensureSystemPrompt(messages)
	messages = append(messages, provider.LLMMessage{Role: "user", Content: userInput})
	if err := e.sessionMgr.SaveUserMessage(sessionID, userInput); err != nil {
		stream <- StreamChunk{Type: "error", Error: "保存消息失败: " + err.Error()}
		return err
	}

	return e.runReActLoop(ctx, sessionID, messages, stream)
}

func (e *Engine) RunWithConfirm(ctx context.Context, sessionID uint, toolCallID string, confirmed bool, stream chan<- StreamChunk) (err error) {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Engine] panic recovered in RunWithConfirm: %v", r)
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

	var targetTC *provider.ToolCall
	var targetTCMsgIdx int = -1
	for i, m := range messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for j := range m.ToolCalls {
				if m.ToolCalls[j].ID == toolCallID {
					targetTC = &m.ToolCalls[j]
					targetTCMsgIdx = i
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

	var resultContent string
	if !confirmed {
		resultContent = "用户取消了此操作"
	} else {
		stream <- StreamChunk{
			Type:       "tool_call",
			ToolCallID: targetTC.ID,
			ToolName:   targetTC.Function.Name,
			Content:    targetTC.Function.Arguments,
		}

		toolResult := e.executor.Execute(ctx, *targetTC)
		resultContent = e.formatToolResult(toolResult)

		stream <- StreamChunk{
			Type:       "tool_result",
			ToolCallID: targetTC.ID,
			ToolName:   targetTC.Function.Name,
			Content:    resultContent,
			Success:    toolResult.Success,
		}
	}

	_ = e.sessionMgr.SaveToolResult(sessionID, targetTC.ID, targetTC.Function.Name, resultContent)

	if targetTCMsgIdx >= 0 && targetTCMsgIdx < len(messages) {
		messages = append(messages[:targetTCMsgIdx+1], provider.LLMMessage{
			Role:       "tool",
			ToolCallID: targetTC.ID,
			Content:    resultContent,
		})
	}

	messages = e.ensureSystemPrompt(messages)
	return e.runReActLoop(ctx, sessionID, messages, stream)
}

func (e *Engine) runReActLoop(ctx context.Context, sessionID uint, messages []provider.LLMMessage, stream chan<- StreamChunk) error {
	toolDefs := e.registry.ToDefinitions()
	consecutiveErrors := 0

	for step := 0; step < e.maxSteps; step++ {
		global.LOG.Debugf("[Engine] ReAct step %d/%d", step+1, e.maxSteps)

		resp, err := e.provider.Chat(ctx, messages, toolDefs)
		if err != nil {
			global.LOG.Errorf("[Engine] LLM call failed: %v", err)
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

			toolResult := e.executor.Execute(ctx, tc)
			resultContent := e.formatToolResult(toolResult)

			if !toolResult.Success && strings.Contains(toolResult.Error, "confirm required") {
				stream <- StreamChunk{
					Type:       "confirm_required",
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					Message:    toolResult.Error,
				}
				stream <- StreamChunk{Type: "done", Success: false, Error: "等待用户确认"}
				return nil
			}

			stream <- StreamChunk{
				Type:       "tool_result",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    resultContent,
				Success:    toolResult.Success,
			}

			toolResults = append(toolResults, provider.LLMMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    resultContent,
			})
			_ = e.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, resultContent)

			if !toolResult.Success {
				consecutiveErrors++
				global.LOG.Warnf("[Engine] tool %s failed: %s", tc.Function.Name, toolResult.Error)
			} else {
				consecutiveErrors = 0
			}
		}

		if consecutiveErrors >= e.maxErrors {
			errMsg := fmt.Sprintf("工具连续执行失败 %d 次，已终止。请检查参数或尝试其他方式。", consecutiveErrors)
			global.LOG.Errorf("[Engine] %s", errMsg)
			stream <- StreamChunk{Type: "error", Error: errMsg}
			stream <- StreamChunk{Type: "done", Success: false}
			return fmt.Errorf(errMsg)
		}

		messages = append(messages, toolResults...)
		messages = e.sessionMgr.CompressIfNeeded(messages)
	}

	stream <- StreamChunk{Type: "error", Error: "达到最大思考步数限制"}
	stream <- StreamChunk{Type: "done", Success: false}
	return fmt.Errorf("max steps exceeded")
}

func (e *Engine) ensureSystemPrompt(messages []provider.LLMMessage) []provider.LLMMessage {
	if e.systemPrompt == "" {
		return messages
	}
	for _, m := range messages {
		if m.Role == "system" {
			return messages
		}
	}
	return append([]provider.LLMMessage{{Role: "system", Content: e.systemPrompt}}, messages...)
}

func (e *Engine) formatToolResult(result tools.ToolResult) string {
	if result.Success {
		if result.Result == "" {
			return "执行成功（无输出）"
		}
		return result.Result
	}
	if result.Result != "" {
		return fmt.Sprintf("执行失败: %s\n输出:\n%s", result.Error, result.Result)
	}
	return fmt.Sprintf("执行失败: %s", result.Error)
}

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

func BuildTitleFromContent(content string) string {
	if len(content) <= 20 {
		return content
	}
	return content[:20] + "..."
}

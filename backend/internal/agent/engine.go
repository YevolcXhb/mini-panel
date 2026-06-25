package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

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

const maxToolOutputLength = 4000

type recentToolCall struct {
	Name string
	Args string
}

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
		maxErrors:    5,
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
	var recentCalls []recentToolCall

	for step := 0; step < e.maxSteps; step++ {
		global.LOG.Debugf("[Engine] ReAct step %d/%d, errors=%d", step+1, e.maxSteps, consecutiveErrors)

		resp, err := e.chatWithRetry(ctx, messages, toolDefs, 3)
		if err != nil {
			global.LOG.Errorf("[Engine] LLM call failed after retries: %v", err)
			stream <- StreamChunk{Type: "error", Error: "LLM 调用失败: " + err.Error()}
			return err
		}

		if len(resp.ToolCalls) == 0 {
			content := resp.Content
			if strings.TrimSpace(content) == "" {
				content = "工具执行完成，结果已展示。如果需要进一步操作，请告诉我。"
			}
			stream <- StreamChunk{Type: "message", Content: content}
			messages = append(messages, provider.LLMMessage{Role: "assistant", Content: content})
			_ = e.sessionMgr.SaveAssistantMessage(sessionID, content, nil)
			stream <- StreamChunk{Type: "done", Success: true}
			return nil
		}

		if strings.TrimSpace(resp.Content) != "" {
			stream <- StreamChunk{Type: "message", Content: resp.Content}
		}

		messages = append(messages, provider.LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, resp.ToolCalls)

		var toolResults []provider.LLMMessage

		for _, tc := range resp.ToolCalls {
			argsStr := tc.Function.Arguments
			callKey := recentToolCall{Name: tc.Function.Name, Args: argsStr}

			dupCount := 0
			for _, rc := range recentCalls {
				if rc.Name == callKey.Name && rc.Args == callKey.Args {
					dupCount++
				}
			}
			if dupCount >= 2 {
				warnMsg := fmt.Sprintf("注意：你已经连续%d次调用相同的工具 %s (参数: %s)，请换一种思路或工具，不要重复相同操作。", dupCount+1, tc.Function.Name, argsStr)
				global.LOG.Warnf("[Engine] %s", warnMsg)
				resultContent := warnMsg
				toolResults = append(toolResults, provider.LLMMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    resultContent,
				})
				_ = e.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, resultContent)
				consecutiveErrors++
				continue
			}

			stream <- StreamChunk{
				Type:       "tool_call",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    argsStr,
			}

			toolResult := e.executor.Execute(ctx, tc)
			resultContent := e.formatToolResult(toolResult)

			if len(resultContent) > maxToolOutputLength {
				resultContent = resultContent[:maxToolOutputLength] + fmt.Sprintf("\n... [输出过长，已截断。总长度 %d 字符]", len(resultContent))
			}

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

			recentCalls = append(recentCalls, callKey)
			if len(recentCalls) > 5 {
				recentCalls = recentCalls[1:]
			}

			if !toolResult.Success {
				consecutiveErrors++
				global.LOG.Warnf("[Engine] tool %s failed (consecutive=%d): %s", tc.Function.Name, consecutiveErrors, toolResult.Error)
			} else {
				consecutiveErrors = 0
			}
		}

		if consecutiveErrors >= e.maxErrors {
			global.LOG.Warnf("[Engine] 连续 %d 次错误，尝试强制总结", consecutiveErrors)
			messages = append(messages, toolResults...)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "工具连续执行多次失败。请停止调用工具，直接总结当前已获取的信息，说明遇到的问题，并给用户建议。不要继续调用工具。",
			})
			break
		}

		if step >= 15 && step < e.maxSteps-1 {
			global.LOG.Infof("[Engine] 已执行 %d 步，提示模型尽快收尾", step+1)
			messages = append(messages, toolResults...)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "注意：你已经执行了多个步骤。如果核心任务已经完成，请立即总结结果回复用户，不要做过多无关的检查和验证步骤。如果还需要关键工具才能完成，可以继续，但请尽快收尾。",
			})
			messages = e.sessionMgr.CompressIfNeeded(messages)
			continue
		}

		if step >= e.maxSteps-1 {
			global.LOG.Warnf("[Engine] 达到最大步数 %d，强制总结", e.maxSteps)
			messages = append(messages, toolResults...)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "已达到最大思考步数。请立即停止调用工具，对当前执行结果给出总结，说明完成了什么，还有什么没完成，给用户建议。",
			})
			break
		}

		messages = append(messages, toolResults...)
		messages = e.sessionMgr.CompressIfNeeded(messages)
	}

	toolDefsEmpty := []provider.ToolDefinition{}
	finalResp, err := e.chatWithRetry(ctx, messages, toolDefsEmpty, 2)
	if err != nil {
		global.LOG.Errorf("[Engine] 最终总结调用失败: %v", err)
		stream <- StreamChunk{Type: "message", Content: "操作已执行，但生成总结时遇到问题。请查看上方工具执行结果。"}
		stream <- StreamChunk{Type: "done", Success: true}
		return nil
	}

	content := finalResp.Content
	if strings.TrimSpace(content) == "" {
		content = "工具执行完成，结果已展示。如果需要进一步操作，请告诉我。"
	}
	stream <- StreamChunk{Type: "message", Content: content}
	_ = e.sessionMgr.SaveAssistantMessage(sessionID, content, nil)
	stream <- StreamChunk{Type: "done", Success: true}
	return nil
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

func (e *Engine) chatWithRetry(ctx context.Context, messages []provider.LLMMessage, tools []provider.ToolDefinition, maxRetries int) (*provider.LLMResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			waitMs := int(math.Pow(2, float64(attempt-1))) * 1000
			global.LOG.Warnf("[Engine] LLM 调用限流，等待 %dms 后重试 (第%d次)", waitMs, attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(waitMs) * time.Millisecond):
			}
		}

		resp, err := e.provider.Chat(ctx, messages, tools)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		errStr := err.Error()
		isRateLimit := strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "Too Many Requests") ||
			strings.Contains(errStr, "rate limit")
		if !isRateLimit || attempt == maxRetries {
			return nil, err
		}
	}
	return nil, lastErr
}

func argsStrToMap(args string) map[string]interface{} {
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(args), &m)
	return m
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

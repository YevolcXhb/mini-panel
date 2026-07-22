package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/compression"
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
	provider        provider.Provider
	registry        *tools.Registry
	executor        *tools.Executor
	sessionMgr      *repository.SessionManager
	systemPrompt    string
	maxSteps        int
	maxErrors       int
	microCompressor *compression.MicroCompressionStrategy
}

func NewEngine(p provider.Provider, registry *tools.Registry, systemPrompt string, maxSteps int) *Engine {
	return &Engine{
		provider:        p,
		registry:        registry,
		executor:        tools.NewExecutor(registry),
		sessionMgr:      repository.NewSessionManager(),
		systemPrompt:    systemPrompt,
		maxSteps:        maxSteps,
		maxErrors:       5,
		microCompressor: compression.NewMicroCompressionStrategy(),
	}
}

func (e *Engine) Run(ctx context.Context, sessionID uint, userInput string, stream chan<- StreamChunk) (err error) {
	global.LOG.Infof("[Engine] 会话%d: 🚀 收到新用户消息，开始处理: %s", sessionID, truncateStr(userInput, 100))
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
		global.LOG.Errorf("[Engine] 会话%d: 加载会话失败: %v", sessionID, err)
		stream <- StreamChunk{Type: "error", Error: "加载会话失败: " + err.Error()}
		return err
	}
	global.LOG.Infof("[Engine] 会话%d: 加载历史消息成功，共%d条", sessionID, len(messages))

	messages = e.ensureSystemPrompt(messages)
	messages = append(messages, provider.LLMMessage{Role: "user", Content: userInput})
	if err := e.sessionMgr.SaveUserMessage(sessionID, userInput); err != nil {
		global.LOG.Errorf("[Engine] 会话%d: 保存用户消息失败: %v", sessionID, err)
		stream <- StreamChunk{Type: "error", Error: "保存消息失败: " + err.Error()}
		return err
	}

	return e.runReActLoop(ctx, sessionID, messages, stream)
}

func (e *Engine) RunWithConfirm(ctx context.Context, sessionID uint, toolCallID string, confirmed bool, stream chan<- StreamChunk) (err error) {
	global.LOG.Infof("[Engine] 会话%d: 🚀 收到用户确认，toolCallID=%s, confirmed=%v", sessionID, toolCallID, confirmed)
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
		global.LOG.Errorf("[Engine] 会话%d: 加载会话失败: %v", sessionID, err)
		stream <- StreamChunk{Type: "error", Error: "加载会话失败: " + err.Error()}
		return err
	}
	global.LOG.Infof("[Engine] 会话%d: 加载历史消息成功，共%d条", sessionID, len(messages))

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
		global.LOG.Errorf("[Engine] 会话%d: 找不到toolCallID=%s对应的工具调用", sessionID, toolCallID)
		stream <- StreamChunk{Type: "error", Error: "找不到工具调用信息"}
		return fmt.Errorf("tool call not found: %s", toolCallID)
	}
	global.LOG.Infof("[Engine] 会话%d: 找到待确认工具调用: %s", sessionID, targetTC.Function.Name)

	var resultContent string
	if !confirmed {
		global.LOG.Infof("[Engine] 会话%d: 用户取消了操作", sessionID)
		resultContent = "用户取消了此操作"
	} else {
		global.LOG.Infof("[Engine] 会话%d: 用户确认执行，开始执行工具: %s", sessionID, targetTC.Function.Name)
		stream <- StreamChunk{
			Type:       "tool_call",
			ToolCallID: targetTC.ID,
			ToolName:   targetTC.Function.Name,
			Content:    targetTC.Function.Arguments,
		}

		// 用户已确认：临时允许危险命令执行（仅本次调用）
		prevOpts := tools.GetExecOptionsForTest()
		tools.SetExecOptions(tools.ExecOptions{
			AllowDangerous: true,
			Timeout:        prevOpts.Timeout,
		})
		toolResult := e.executor.Execute(ctx, *targetTC)
		// 恢复原配置，避免影响后续同连接内的其他请求
		tools.SetExecOptions(prevOpts)
		resultContent = e.formatToolResult(toolResult)
		global.LOG.Infof("[Engine] 会话%d: 确认执行的工具%s完成，成功=%v，结果长度=%d",
			sessionID, targetTC.Function.Name, toolResult.Success, len(resultContent))

		stream <- StreamChunk{
			Type:       "tool_result",
			ToolCallID: targetTC.ID,
			ToolName:   targetTC.Function.Name,
			Content:    resultContent,
			Success:    toolResult.Success,
		}
	}

	_ = e.sessionMgr.SaveToolResult(sessionID, targetTC.ID, targetTC.Function.Name, resultContent)
	global.LOG.Infof("[Engine] 会话%d: 工具结果已保存，继续ReAct循环", sessionID)

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
	lastCompressionStep := 0
	var recentCalls []recentToolCall
	finalMessageSent := false

	global.LOG.Infof("[Engine] 会话%d: 开始ReAct循环，初始消息数=%d，最大步数=%d", sessionID, len(messages), e.maxSteps)

	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Engine] 会话%d: panic: %v", sessionID, r)
		}
		if !finalMessageSent {
			global.LOG.Warnf("[Engine] 会话%d: ⚠️ 触发兜底逻辑，发送默认最终消息", sessionID)
			stream <- StreamChunk{Type: "message", Content: "操作已执行完成，请查看上方结果。如果有其他问题，请随时告诉我。"}
			stream <- StreamChunk{Type: "done", Success: true}
		} else {
			global.LOG.Infof("[Engine] 会话%d: ✅ 循环正常结束，已发送最终消息", sessionID)
		}
	}()

	for step := 0; step < e.maxSteps; step++ {
		select {
		case <-ctx.Done():
			global.LOG.Infof("[Engine] 会话%d: 上下文取消，退出循环", sessionID)
			// 保存中断标记：如果最后一条 assistant 消息含 tool_calls 但缺少 tool result，
			// 补充虚拟 result 到数据库，避免下次对话时 LLM API 400 错误
			e.saveInterruptedToolResults(sessionID, messages)
			return ctx.Err()
		default:
		}

		global.LOG.Infof("[Engine] 会话%d: ====== 第%d/%d步，开始调用LLM，当前消息数=%d，连续错误=%d ======",
			sessionID, step+1, e.maxSteps, len(messages), consecutiveErrors)

		resp, err := e.chatStreamWithRetry(ctx, messages, toolDefs, 3, stream, "")
		if err != nil {
			global.LOG.Errorf("[Engine] 会话%d: ❌ LLM调用失败: %v", sessionID, err)
			stream <- StreamChunk{Type: "error", Error: "LLM 调用失败: " + err.Error()}
			return err
		}

		hasToolCalls := len(resp.ToolCalls) > 0
		contentLen := len(strings.TrimSpace(resp.Content))
		global.LOG.Infof("[Engine] 会话%d: LLM返回: 有工具调用=%v，文本内容长度=%d，工具调用数量=%d",
			sessionID, hasToolCalls, contentLen, len(resp.ToolCalls))

		if contentLen > 0 {
			global.LOG.Debugf("[Engine] 会话%d: LLM文本内容预览: %s", sessionID, truncateStr(resp.Content, 100))
		}

		if !hasToolCalls {
			content := resp.Content
			if strings.TrimSpace(content) == "" {
				global.LOG.Warnf("[Engine] 会话%d: LLM无工具调用且内容为空，使用默认回复", sessionID)
				content = "工具执行完成，结果已展示。如果需要进一步操作，请告诉我。"
				stream <- StreamChunk{Type: "message", Content: content}
			} else {
				global.LOG.Infof("[Engine] 会话%d: ✅ LLM无工具调用，直接发送最终回复", sessionID)
				// 流式路径已逐 token 推送，此处无需再推 message
			}
			messages = append(messages, provider.LLMMessage{Role: "assistant", Content: content})
			_ = e.sessionMgr.SaveAssistantMessage(sessionID, content, nil)
			finalMessageSent = true
			stream <- StreamChunk{Type: "done", Success: true}
			global.LOG.Infof("[Engine] 会话%d: ✅ 已发送done事件，流程结束", sessionID)
			return nil
		}

		// 流式路径已逐 token 推送中间说明文本，此处无需再推 message
		messages = append(messages, provider.LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, resp.ToolCalls)

		var toolResults []provider.LLMMessage

		for i, tc := range resp.ToolCalls {
			argsStr := tc.Function.Arguments
			global.LOG.Infof("[Engine] 会话%d: 🔧 开始执行第%d/%d个工具: %s", sessionID, i+1, len(resp.ToolCalls), tc.Function.Name)
			global.LOG.Debugf("[Engine] 会话%d: 工具%s参数: %s", sessionID, tc.Function.Name, truncateStr(argsStr, 200))

			callKey := recentToolCall{Name: tc.Function.Name, Args: argsStr}

			dupCount := 0
			for _, rc := range recentCalls {
				if rc.Name == callKey.Name && rc.Args == callKey.Args {
					dupCount++
				}
			}
			if dupCount >= 2 {
				warnMsg := fmt.Sprintf("注意：你已经连续%d次调用相同的工具 %s (参数: %s)，请换一种思路或工具，不要重复相同操作。", dupCount+1, tc.Function.Name, argsStr)
				global.LOG.Warnf("[Engine] 会话%d: ⚠️ 检测到重复调用工具%s %d次，发送警告", sessionID, tc.Function.Name, dupCount+1)
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
			global.LOG.Infof("[Engine] 会话%d: 工具%s执行完成，成功=%v，结果长度=%d",
				sessionID, tc.Function.Name, toolResult.Success, len(resultContent))
			global.LOG.Debugf("[Engine] 会话%d: 工具%s结果预览: %s", sessionID, tc.Function.Name, truncateStr(resultContent, 200))

			if len(resultContent) > maxToolOutputLength {
				resultContent = resultContent[:maxToolOutputLength] + fmt.Sprintf("\n... [输出过长，已截断。总长度 %d 字符]", len(resultContent))
				global.LOG.Warnf("[Engine] 会话%d: 工具输出过长，已截断到%d字符", sessionID, maxToolOutputLength)
			}

			if !toolResult.Success && strings.Contains(toolResult.Error, "confirm required") {
				global.LOG.Infof("[Engine] 会话%d: ⚠️ 工具%s需要用户确认，暂停等待", sessionID, tc.Function.Name)
				stream <- StreamChunk{
					Type:       "confirm_required",
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					Message:    toolResult.Error,
				}
				finalMessageSent = true
				stream <- StreamChunk{Type: "done", Success: false, Error: "等待用户确认"}
				global.LOG.Infof("[Engine] 会话%d: ✅ 已发送等待确认done事件", sessionID)
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
				global.LOG.Warnf("[Engine] 会话%d: 工具%s失败 (连续错误=%d): %s",
					sessionID, tc.Function.Name, consecutiveErrors, toolResult.Error)
			} else {
				consecutiveErrors = 0
			}
		}

		messages = append(messages, toolResults...)
		global.LOG.Infof("[Engine] 会话%d: 本轮%d个工具执行完成，追加结果到上下文，当前消息总数=%d",
			sessionID, len(toolResults), len(messages))

		if consecutiveErrors >= e.maxErrors {
			global.LOG.Warnf("[Engine] 会话%d: 🔴 连续%d次错误，强制总结，不再调用工具", sessionID, consecutiveErrors)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "工具连续执行多次失败。请停止调用工具，直接总结当前已获取的信息，说明遇到的问题，并给用户建议。不要继续调用工具。",
			})
			break
		}

		if step >= e.maxSteps-2 {
			global.LOG.Warnf("[Engine] 会话%d: 🟡 达到步数限制(%d步)，强制总结，不再调用工具", sessionID, step+1)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "请立即停止调用工具，对当前执行结果给出总结，说明完成了什么，给用户明确的答复。",
			})
			break
		}

		// 微压缩检查：双触发（语义边界 ∨ 步数/错误阈值）
		compCtx := compression.CompressionContext{
			StepNumber:          step + 1,
			MessageCount:        len(messages),
			ConsecutiveErrors:   consecutiveErrors,
			PhaseName:           "react",
			LastMessage:         e.getLastAssistantContent(messages),
			LastCompressionStep: lastCompressionStep,
		}
		if e.microCompressor.ShouldCompress(compCtx) {
			var report compression.CompressionReport
			messages, report = e.microCompressor.Compress(messages, compCtx)
			lastCompressionStep = step + 1
			global.LOG.Infof("[Engine] 会话%d: 压缩触发 trigger=%s, tokens_saved=%d, messages=%d→%d",
				sessionID, report.Trigger, report.TokensSaved, report.MessagesCompressed, len(messages))
			stream <- StreamChunk{
				Type:        "compression_triggered",
				TokensSaved: report.TokensSaved,
				Content:     string(report.Trigger),
			}
		}
		global.LOG.Infof("[Engine] 会话%d: 消息数量=%d，进入下一轮循环", sessionID, len(messages))
	}

	global.LOG.Infof("[Engine] 会话%d: ====== 开始生成最终总结，不传递工具定义 ======", sessionID)
	toolDefsEmpty := []provider.ToolDefinition{}
	finalResp, err := e.chatStreamWithRetry(ctx, messages, toolDefsEmpty, 2, stream, "")
	if err != nil {
		global.LOG.Errorf("[Engine] 会话%d: ❌ 最终总结LLM调用失败: %v，发送兜底消息", sessionID, err)
		stream <- StreamChunk{Type: "message", Content: "操作已执行，但生成总结时遇到问题。请查看上方工具执行结果。如果需要进一步分析，请告诉我。"}
	} else {
		content := finalResp.Content
		contentLen := len(strings.TrimSpace(content))
		global.LOG.Infof("[Engine] 会话%d: 最终总结LLM返回，内容长度=%d", sessionID, contentLen)
		if contentLen == 0 {
			global.LOG.Warnf("[Engine] 会话%d: 最终总结内容为空，使用默认回复", sessionID)
			content = "工具执行完成，结果已展示。如果需要进一步操作，请告诉我。"
			stream <- StreamChunk{Type: "message", Content: content}
		} else {
			global.LOG.Debugf("[Engine] 会话%d: 最终总结预览: %s", sessionID, truncateStr(content, 200))
			// 流式路径已逐 token 推送，此处无需再推 message
		}
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, content, nil)
	}
	finalMessageSent = true
	stream <- StreamChunk{Type: "done", Success: true}
	global.LOG.Infof("[Engine] 会话%d: ✅ 最终总结和done事件已发送，流程全部结束", sessionID)
	return nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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

// getLastAssistantContent 获取消息列表中最后一条 assistant 消息的内容（用于压缩语义触发检测）
func (e *Engine) getLastAssistantContent(messages []provider.LLMMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
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

// saveInterruptedToolResults 检查内存中的消息列表，如果最后一条 assistant 消息
// 含有 tool_calls 但缺少对应的 tool result（说明用户中断了回复），
// 则将虚拟中断 result 保存到数据库，避免下次对话时 LLM API 报 400 错误。
func (e *Engine) saveInterruptedToolResults(sessionID uint, messages []provider.LLMMessage) {
	if len(messages) == 0 {
		return
	}

	// 收集已有的 tool_call_id
	toolResults := make(map[string]bool)
	for _, m := range messages {
		if m.Role == "tool" && m.ToolCallID != "" {
			toolResults[m.ToolCallID] = true
		}
	}

	// 检查所有 assistant 消息的 tool_calls
	for _, m := range messages {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if !toolResults[tc.ID] {
				global.LOG.Warnf("[Engine] 会话%d: 检测到中断的 tool_call(id=%s, name=%s)，保存中断标记",
					sessionID, tc.ID, tc.Function.Name)
				_ = e.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, "[操作已中断：用户停止了回复]")
			}
		}
	}
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

// chatStreamWithRetry 流式调用 LLM，边收 token 边推送到前端 stream。
// 若 Provider 未实现 Streamer 接口，自动回退到非流式 chatWithRetry（推送 Type="message"）。
// 返回聚合后的 *LLMResponse，保持 ReAct 循环逻辑不变。
// phaseLabel 仅用于日志标识（如 "planning"），可为空。
func (e *Engine) chatStreamWithRetry(ctx context.Context, messages []provider.LLMMessage, tools []provider.ToolDefinition, maxRetries int, stream chan<- StreamChunk, phaseLabel string) (*provider.LLMResponse, error) {
	streamer, ok := e.provider.(provider.Streamer)
	if !ok {
		// 回退：非流式调用，并一次性推送完整内容作为 message 事件
		global.LOG.Infof("[Engine] Provider 未实现 Streamer，回退非流式调用%s", logPhase(phaseLabel))
		resp, err := e.chatWithRetry(ctx, messages, tools, maxRetries)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(resp.Content) != "" {
			stream <- StreamChunk{Type: "message", Content: resp.Content, Phase: phaseLabel}
		}
		return resp, nil
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			waitMs := int(math.Pow(2, float64(attempt-1))) * 1000
			global.LOG.Warnf("[Engine] 流式调用限流，等待 %dms 后重试 (第%d次)%s", waitMs, attempt, logPhase(phaseLabel))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(waitMs) * time.Millisecond):
			}
		}

		deltaCh, err := streamer.ChatStream(ctx, messages, tools)
		if err != nil {
			lastErr = err
			errStr := err.Error()
			isRateLimit := strings.Contains(errStr, "429") ||
				strings.Contains(errStr, "Too Many Requests") ||
				strings.Contains(errStr, "rate limit")
			if !isRateLimit || attempt == maxRetries {
				return nil, err
			}
			continue
		}

		// 聚合 LLMResponse
		resp := &provider.LLMResponse{}
		var toolCalls []provider.ToolCall
		tokenCount := 0

		for delta := range deltaCh {
			switch delta.Type {
			case "content":
				resp.Content += delta.Content
				tokenCount++
				// 真正的逐 token 推送到前端
				stream <- StreamChunk{Type: "token", Content: delta.Content, Phase: phaseLabel}
			case "tool_call":
				tc := provider.ToolCall{
					ID:   delta.ToolCallID,
					Type: delta.ToolCallType,
				}
				tc.Function.Name = delta.FunctionName
				tc.Function.Arguments = delta.ArgumentsDelta
				toolCalls = append(toolCalls, tc)
			case "usage":
				if delta.Usage != nil {
					resp.Usage = *delta.Usage
				}
			case "error":
				// 流内部错误，作为本次调用失败处理
				lastErr = fmt.Errorf("stream error: %s", delta.Content)
				global.LOG.Errorf("[Engine] 流式响应内部错误%s: %s", logPhase(phaseLabel), delta.Content)
				break
			case "done":
				// 流正常结束
			}
			if delta.Type == "error" {
				break
			}
		}

		if lastErr != nil {
			errStr := lastErr.Error()
			isRateLimit := strings.Contains(errStr, "429") ||
				strings.Contains(errStr, "Too Many Requests") ||
				strings.Contains(errStr, "rate limit")
			if !isRateLimit || attempt == maxRetries {
				return nil, lastErr
			}
			continue
		}

		resp.ToolCalls = toolCalls
		global.LOG.Infof("[Engine] 流式调用完成%s，token分片数=%d，内容长度=%d，工具调用数=%d",
			logPhase(phaseLabel), tokenCount, len(resp.Content), len(resp.ToolCalls))
		return resp, nil
	}
	return nil, lastErr
}

// logPhase 返回带阶段标签的日志后缀（带前导空格），phaseLabel 为空时返回空串
func logPhase(phaseLabel string) string {
	if phaseLabel == "" {
		return ""
	}
	return " [phase=" + phaseLabel + "]"
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

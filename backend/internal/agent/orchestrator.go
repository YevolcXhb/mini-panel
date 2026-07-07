package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/minipanel/minipanel/internal/agent/compression"
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
)

// PendingPlan 等待用户确认的计划
type PendingPlan struct {
	Task            string
	Plan            string
	PlanningSummary compression.SessionSummary // PLANNING 阶段的结构化摘要，用于 CODING handoff
	Phase           OrchestratorPhase
	Created         time.Time
}

// Orchestrator 三阶段编排器
type Orchestrator struct {
	engine            *Engine
	sessionCompressor *compression.SessionCompressionStrategy

	// pendingPlans 存储等待确认的计划（sessionID → PendingPlan）
	mu           sync.Mutex
	pendingPlans map[uint]*PendingPlan
}

// NewOrchestrator 创建编排器
func NewOrchestrator(engine *Engine) *Orchestrator {
	return &Orchestrator{
		engine:            engine,
		sessionCompressor: compression.NewSessionCompressionStrategy(),
		pendingPlans:      make(map[uint]*PendingPlan),
	}
}

// Orchestrate 执行三阶段编排（PLANNING → 等待确认 → CODING → REVIEWING）
func (o *Orchestrator) Orchestrate(ctx context.Context, sessionID uint, task string, stream chan<- StreamChunk) (err error) {
	global.LOG.Infof("[Orchestrator] 会话%d: 🚀 开始三阶段编排，任务: %s", sessionID, truncateStr(task, 100))
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Orchestrator] 会话%d: panic: %v", sessionID, r)
			err = fmt.Errorf("orchestrator panic: %v", r)
			stream <- StreamChunk{Type: "error", Error: fmt.Sprintf("编排器内部错误: %v", r)}
			stream <- StreamChunk{Type: "done", Success: false}
		}
	}()

	// 保存用户消息
	if err := o.engine.sessionMgr.SaveUserMessage(sessionID, task); err != nil {
		global.LOG.Errorf("[Orchestrator] 会话%d: 保存用户消息失败: %v", sessionID, err)
	}

	// Phase 1: PLANNING
	stream <- StreamChunk{Type: "phase_start", Phase: string(PhasePlanning), MaxSteps: MaxStepsPerPhase}
	plan, planningMessages, err := o.runPhase(ctx, sessionID, PhasePlanning, PLANNER_SYSTEM_PROMPT, task, stream)
	if err != nil {
		return err
	}
	stream <- StreamChunk{Type: "phase_complete", Phase: string(PhasePlanning)}

	// 阶段交接：对 PLANNING 阶段消息生成结构化摘要，注入 pendingPlan 供 ConfirmPlan 使用
	planningSummary := o.sessionCompressor.BuildSummary(planningMessages, string(PhasePlanning))
	global.LOG.Infof("[Orchestrator] 会话%d: 规划阶段完成，计划长度=%d, 摘要维度: achievements=%d decisions=%d trials=%d",
		sessionID, len(plan), len(planningSummary.KeyAchievements), len(planningSummary.DesignDecisions), len(planningSummary.TrialPaths))

	// 发送 plan_ready 事件，等待用户确认
	o.mu.Lock()
	o.pendingPlans[sessionID] = &PendingPlan{
		Task:            task,
		Plan:            plan,
		PlanningSummary: planningSummary,
		Phase:           PhasePlanning,
		Created:         time.Now(),
	}
	o.mu.Unlock()

	stream <- StreamChunk{
		Type:    "plan_ready",
		Plan:    plan,
		Message: "规划阶段已完成，请审查以上计划。确认后将开始执行。",
	}
	stream <- StreamChunk{Type: "done", Success: false, Error: "等待用户确认计划"}
	global.LOG.Infof("[Orchestrator] 会话%d: 已发送 plan_ready，等待用户确认", sessionID)

	return nil
}

// ConfirmPlan 用户确认计划后继续执行 CODING 和 REVIEWING 阶段
func (o *Orchestrator) ConfirmPlan(ctx context.Context, sessionID uint, confirmed bool, stream chan<- StreamChunk) (err error) {
	global.LOG.Infof("[Orchestrator] 会话%d: 收到计划确认，confirmed=%v", sessionID, confirmed)
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Orchestrator] 会话%d: panic in ConfirmPlan: %v", sessionID, r)
			err = fmt.Errorf("orchestrator panic: %v", r)
			stream <- StreamChunk{Type: "error", Error: fmt.Sprintf("编排器内部错误: %v", r)}
			stream <- StreamChunk{Type: "done", Success: false}
		}
	}()

	o.mu.Lock()
	pending := o.pendingPlans[sessionID]
	if pending != nil {
		delete(o.pendingPlans, sessionID)
	}
	o.mu.Unlock()

	if pending == nil {
		errMsg := "找不到待确认的计划，可能已超时或未发起编排"
		global.LOG.Errorf("[Orchestrator] 会话%d: %s", sessionID, errMsg)
		stream <- StreamChunk{Type: "error", Error: errMsg}
		return fmt.Errorf(errMsg)
	}

	if !confirmed {
		global.LOG.Infof("[Orchestrator] 会话%d: 用户取消了计划", sessionID)
		cancelMsg := "用户取消了执行计划。如需重新规划，请重新描述任务。"
		_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, cancelMsg, nil)
		stream <- StreamChunk{Type: "message", Content: cancelMsg}
		stream <- StreamChunk{Type: "done", Success: true}
		return nil
	}

	task := pending.Task
	plan := pending.Plan
	planningSummary := pending.PlanningSummary

	// Phase 2: CODING — 注入 PLANNING 阶段的结构化摘要到 handoff context
	codingContext := BuildCodingContext(task, plan)
	if planningSummary.RawSummary != "" {
		codingContext += "\n\n## 规划阶段结构化摘要\n" + planningSummary.RawSummary
	}
	stream <- StreamChunk{Type: "phase_start", Phase: string(PhaseCoding), MaxSteps: MaxStepsPerPhase}
	codeResult, codingMessages, err := o.runPhase(ctx, sessionID, PhaseCoding, CODER_SYSTEM_PROMPT, codingContext, stream)
	if err != nil {
		return err
	}
	stream <- StreamChunk{Type: "phase_complete", Phase: string(PhaseCoding)}

	// 阶段交接：对 CODING 阶段消息生成结构化摘要，注入 REVIEWING handoff
	codingSummary := o.sessionCompressor.BuildSummary(codingMessages, string(PhaseCoding))
	global.LOG.Infof("[Orchestrator] 会话%d: 执行阶段完成，结果长度=%d, 摘要维度: achievements=%d decisions=%d trials=%d",
		sessionID, len(codeResult), len(codingSummary.KeyAchievements), len(codingSummary.DesignDecisions), len(codingSummary.TrialPaths))

	// Phase 3: REVIEWING — 注入 CODING 阶段的结构化摘要到 handoff context
	reviewContext := BuildReviewContext(task, codeResult)
	if codingSummary.RawSummary != "" {
		reviewContext += "\n\n## 执行阶段结构化摘要\n" + codingSummary.RawSummary
	}
	stream <- StreamChunk{Type: "phase_start", Phase: string(PhaseReviewing), MaxSteps: MaxStepsPerPhase}
	reviewResult, _, err := o.runPhase(ctx, sessionID, PhaseReviewing, REVIEWER_SYSTEM_PROMPT, reviewContext, stream)
	if err != nil {
		return err
	}
	stream <- StreamChunk{Type: "phase_complete", Phase: string(PhaseReviewing)}
	global.LOG.Infof("[Orchestrator] 会话%d: 审查阶段完成，结果长度=%d", sessionID, len(reviewResult))

	// 发送最终总结
	finalSummary := fmt.Sprintf("## 三阶段编排完成\n\n### 执行计划\n%s\n\n### 执行结果\n%s\n\n### 审查结论\n%s",
		truncateStr(plan, 500), truncateStr(codeResult, 500), reviewResult)
	_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, finalSummary, nil)
	stream <- StreamChunk{Type: "message", Content: finalSummary}
	stream <- StreamChunk{Type: "done", Success: true}
	global.LOG.Infof("[Orchestrator] 会话%d: ✅ 三阶段编排全部完成", sessionID)

	return nil
}

// runPhase 执行单个阶段的 ReAct 循环。
// 返回值：phaseOutput 阶段产出文本，phaseMessages 该阶段累积的消息列表（用于 sessionCompressor 生成摘要），err 错误。
func (o *Orchestrator) runPhase(ctx context.Context, sessionID uint, phase OrchestratorPhase,
	systemPrompt, handoffContext string, stream chan<- StreamChunk) (phaseOutput string, phaseMessages []provider.LLMMessage, err error) {

	global.LOG.Infof("[Orchestrator] 会话%d: 开始 %s 阶段", sessionID, phase)

	// 构建该阶段的工具定义（白名单过滤）
	allToolDefs := o.engine.registry.ToDefinitions()
	phaseToolDefs := filterToolsByPhase(phase, allToolDefs)

	// 初始化阶段消息（fresh start）
	messages := []provider.LLMMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: handoffContext},
	}

	consecutiveErrors := 0
	lastCompressionStep := 0
	var recentCalls []recentToolCall

	for step := 0; step < MaxStepsPerPhase; step++ {
		select {
		case <-ctx.Done():
			return "", messages, ctx.Err()
		default:
		}

		global.LOG.Infof("[Orchestrator] 会话%d: [%s] 第%d/%d步, 消息数=%d, 错误=%d",
			sessionID, phase, step+1, MaxStepsPerPhase, len(messages), consecutiveErrors)

		// 微压缩检查
		compCtx := compression.CompressionContext{
			StepNumber:          step + 1,
			MessageCount:        len(messages),
			ConsecutiveErrors:   consecutiveErrors,
			PhaseName:           string(phase),
			LastMessage:         o.getLastAssistantContent(messages),
			LastCompressionStep: lastCompressionStep,
		}
		if o.engine.microCompressor.ShouldCompress(compCtx) {
			var report compression.CompressionReport
			messages, report = o.engine.microCompressor.Compress(messages, compCtx)
			lastCompressionStep = step + 1
			global.LOG.Infof("[Orchestrator] 会话%d: [%s] 压缩触发 trigger=%s, tokens_saved=%d",
				sessionID, phase, report.Trigger, report.TokensSaved)
			stream <- StreamChunk{
				Type:        "compression_triggered",
				Phase:       string(phase),
				TokensSaved: report.TokensSaved,
				Content:     string(report.Trigger),
				StepNumber:  step + 1,
				MaxSteps:    MaxStepsPerPhase,
			}
		}

		// 调用 LLM
		resp, err := o.engine.chatWithRetry(ctx, messages, phaseToolDefs, 3)
		if err != nil {
			global.LOG.Errorf("[Orchestrator] 会话%d: [%s] LLM调用失败: %v", sessionID, phase, err)
			stream <- StreamChunk{Type: "error", Error: "LLM 调用失败: " + err.Error()}
			return "", messages, err
		}

		hasToolCalls := len(resp.ToolCalls) > 0
		contentLen := len(strings.TrimSpace(resp.Content))
		global.LOG.Infof("[Orchestrator] 会话%d: [%s] LLM返回: 有工具调用=%v, 文本长度=%d",
			sessionID, phase, hasToolCalls, contentLen)

		// 推送文本内容到前端
		if contentLen > 0 {
			stream <- StreamChunk{
				Type:       "message",
				Content:    resp.Content,
				Phase:      string(phase),
				StepNumber: step + 1,
				MaxSteps:   MaxStepsPerPhase,
			}
		}

		// 检查阶段完成
		if PhaseComplete(phase, resp) {
			global.LOG.Infof("[Orchestrator] 会话%d: [%s] ✅ 阶段完成检测通过", sessionID, phase)
			_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
			if contentLen == 0 {
				return "(阶段完成，无文本输出)", messages, nil
			}
			return resp.Content, messages, nil
		}

		// 无工具调用且未完成阶段
		if !hasToolCalls {
			global.LOG.Infof("[Orchestrator] 会话%d: [%s] 无工具调用，视为阶段完成", sessionID, phase)
			_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
			if contentLen == 0 {
				return "(阶段完成，无文本输出)", messages, nil
			}
			return resp.Content, messages, nil
		}

		// 保存 assistant 消息
		messages = append(messages, provider.LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, resp.ToolCalls)

		// 执行工具调用
		var toolResults []provider.LLMMessage
		for i, tc := range resp.ToolCalls {
			global.LOG.Infof("[Orchestrator] 会话%d: [%s] 执行工具 %d/%d: %s",
				sessionID, phase, i+1, len(resp.ToolCalls), tc.Function.Name)

			// 重复调用检测
			callKey := recentToolCall{Name: tc.Function.Name, Args: tc.Function.Arguments}
			dupCount := 0
			for _, rc := range recentCalls {
				if rc.Name == callKey.Name && rc.Args == callKey.Args {
					dupCount++
				}
			}
			if dupCount >= 2 {
				warnMsg := fmt.Sprintf("注意：已连续%d次调用相同工具 %s，请换一种思路。", dupCount+1, tc.Function.Name)
				toolResults = append(toolResults, provider.LLMMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    warnMsg,
				})
				_ = o.engine.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, warnMsg)
				consecutiveErrors++
				continue
			}

			stream <- StreamChunk{
				Type:       "tool_call",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    tc.Function.Arguments,
				Phase:      string(phase),
			}

			toolResult := o.engine.executor.Execute(ctx, tc)
			resultContent := o.engine.formatToolResult(toolResult)

			if len(resultContent) > maxToolOutputLength {
				resultContent = resultContent[:maxToolOutputLength] +
					fmt.Sprintf("\n... [输出过长，已截断。总长度 %d 字符]", len(resultContent))
			}

			// 确认机制
			if !toolResult.Success && strings.Contains(toolResult.Error, "confirm required") {
				global.LOG.Infof("[Orchestrator] 会话%d: [%s] 工具%s需要确认", sessionID, phase, tc.Function.Name)
				stream <- StreamChunk{
					Type:       "confirm_required",
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					Message:    toolResult.Error,
				}
				stream <- StreamChunk{Type: "done", Success: false, Error: "等待用户确认"}
				return "", messages, fmt.Errorf("等待用户确认")
			}

			stream <- StreamChunk{
				Type:       "tool_result",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    resultContent,
				Success:    toolResult.Success,
				Phase:      string(phase),
			}

			toolResults = append(toolResults, provider.LLMMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    resultContent,
			})
			_ = o.engine.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, resultContent)

			recentCalls = append(recentCalls, callKey)
			if len(recentCalls) > 5 {
				recentCalls = recentCalls[1:]
			}

			if !toolResult.Success {
				consecutiveErrors++
			} else {
				consecutiveErrors = 0
			}
		}

		messages = append(messages, toolResults...)

		// 连续错误检查
		if consecutiveErrors >= o.engine.maxErrors {
			global.LOG.Warnf("[Orchestrator] 会话%d: [%s] 连续%d次错误，强制结束阶段",
				sessionID, phase, consecutiveErrors)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "工具连续执行多次失败。请停止调用工具，直接总结当前已获取的信息。",
			})
			break
		}

		// 接近步数限制
		if step >= MaxStepsPerPhase-3 {
			global.LOG.Warnf("[Orchestrator] 会话%d: [%s] 接近步数限制(%d)，强制总结",
				sessionID, phase, step+1)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "请立即停止调用工具，对当前执行结果给出总结。",
			})
		}
	}

	// 阶段超步数，调用 LLM 生成总结
	global.LOG.Infof("[Orchestrator] 会话%d: [%s] 阶段达到步数限制，生成总结", sessionID, phase)
	finalResp, err := o.engine.chatWithRetry(ctx, messages, []provider.ToolDefinition{}, 2)
	if err != nil {
		return fmt.Sprintf("[%s phase exceeded max steps]", phase), messages, nil
	}
	content := finalResp.Content
	if strings.TrimSpace(content) == "" {
		content = fmt.Sprintf("[%s phase completed with no output]", phase)
	}
	_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, content, nil)
	return content, messages, nil
}

// filterToolsByPhase 根据阶段白名单过滤工具定义
func filterToolsByPhase(phase OrchestratorPhase, allDefs []provider.ToolDefinition) []provider.ToolDefinition {
	allowed := PhaseToolNames[phase]
	if len(allowed) == 0 {
		// 空集合表示允许所有工具
		return allDefs
	}
	var filtered []provider.ToolDefinition
	for _, def := range allDefs {
		if allowed[def.Function.Name] {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

// getLastAssistantContent 获取最后一条 assistant 消息内容
func (o *Orchestrator) getLastAssistantContent(messages []provider.LLMMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			return messages[i].Content
		}
	}
	return ""
}

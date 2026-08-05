package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/minipanel/minipanel/internal/agent/compression"
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/tools"
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

	// 工具确认暂停时保存的 phase 状态，以及 phase 完成后继续执行的后续步骤
	pendingToolConfirms  map[uint]*pendingToolConfirm
	pendingContinuations map[uint]func(ctx context.Context, stream chan<- StreamChunk, phaseOutput string, phaseMessages []provider.LLMMessage) error
}

// pendingToolConfirm 编排过程中等待用户确认的工具调用及其 phase 状态
type pendingToolConfirm struct {
	Phase          OrchestratorPhase
	SystemPrompt   string
	HandoffContext string
	Messages       []provider.LLMMessage
	State          phaseRunState
}

// phaseRunState 单个 phase 循环的可恢复状态
type phaseRunState struct {
	Step                int
	ConsecutiveErrors   int
	LastCompressionStep int
	RecentCalls         []recentToolCall
}

// NewOrchestrator 创建编排器
func NewOrchestrator(engine *Engine) *Orchestrator {
	return &Orchestrator{
		engine:               engine,
		sessionCompressor:    compression.NewSessionCompressionStrategy(),
		pendingPlans:         make(map[uint]*PendingPlan),
		pendingToolConfirms:  make(map[uint]*pendingToolConfirm),
		pendingContinuations: make(map[uint]func(ctx context.Context, stream chan<- StreamChunk, phaseOutput string, phaseMessages []provider.LLMMessage) error),
	}
}

// Orchestrate 执行三阶段编排（PLANNING → 等待确认 → CODING → REVIEWING）
func (o *Orchestrator) Orchestrate(ctx context.Context, sessionID uint, task string, stream chan<- StreamChunk) (err error) {
	global.LOG.Infof("[Orchestrator] 会话%d: 🚀 开始三阶段编排，任务: %s", sessionID, truncateStr(task, 100))
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("[Orchestrator] 会话%d: panic: %v", sessionID, r)
			err = fmt.Errorf("orchestrator panic: %v", r)
			sendChunk(ctx, stream, StreamChunk{Type: "error", Error: fmt.Sprintf("编排器内部错误: %v", r)})
			sendChunk(ctx, stream, StreamChunk{Type: "done", Success: false})
		}
	}()

	// 保存用户消息
	// 先加载历史消息，避免把刚保存的本次任务重复注入 planning 上下文
	historyMessages, err := o.engine.sessionMgr.LoadMessages(sessionID)
	if err != nil {
		global.LOG.Errorf("[Orchestrator] 会话%d: 加载历史消息失败: %v", sessionID, err)
		historyMessages = nil
	}
	global.LOG.Infof("[Orchestrator] 会话%d: 加载历史消息 %d 条作为上下文", sessionID, len(historyMessages))

	// 保存用户消息
	if err := o.engine.sessionMgr.SaveUserMessage(sessionID, task); err != nil {
		global.LOG.Errorf("[Orchestrator] 会话%d: 保存用户消息失败: %v", sessionID, err)
	}

	// Phase 1: PLANNING
	sendChunk(ctx, stream, StreamChunk{Type: "phase_start", Phase: string(PhasePlanning), MaxSteps: MaxStepsPerPhase})
	plan, planningMessages, err := o.runPhase(ctx, sessionID, PhasePlanning, PLANNER_SYSTEM_PROMPT, task, stream, historyMessages)
	if err != nil {
		return err
	}
	sendChunk(ctx, stream, StreamChunk{Type: "phase_complete", Phase: string(PhasePlanning)})

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

	sendChunk(ctx, stream, StreamChunk{
		Type:    "plan_ready",
		Plan:    plan,
		Message: "规划阶段已完成，请审查以上计划。确认后将开始执行。",
	})
	sendChunk(ctx, stream, StreamChunk{Type: "done", Success: false, Error: "等待用户确认计划"})
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
			sendChunk(ctx, stream, StreamChunk{Type: "error", Error: fmt.Sprintf("编排器内部错误: %v", r)})
			sendChunk(ctx, stream, StreamChunk{Type: "done", Success: false})
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
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: errMsg})
		return fmt.Errorf(errMsg)
	}

	if !confirmed {
		global.LOG.Infof("[Orchestrator] 会话%d: 用户取消了计划", sessionID)
		cancelMsg := "用户取消了执行计划。如需重新规划，请重新描述任务。"
		_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, cancelMsg, nil)
		sendChunk(ctx, stream, StreamChunk{Type: "message", Content: cancelMsg})
		sendChunk(ctx, stream, StreamChunk{Type: "done", Success: true})
		return nil
	}

	task := pending.Task
	plan := pending.Plan
	planningSummary := pending.PlanningSummary

	return o.runCodingAndReview(ctx, sessionID, task, plan, planningSummary, stream)
}

// runCodingAndReview 执行 CODING 阶段；若工具确认暂停，保存后续继续执行的闭包
func (o *Orchestrator) runCodingAndReview(ctx context.Context, sessionID uint, task, plan string,
	planningSummary compression.SessionSummary, stream chan<- StreamChunk) error {

	codingContext := BuildCodingContext(task, plan)
	if planningSummary.RawSummary != "" {
		codingContext += "\n\n## 规划阶段结构化摘要\n" + planningSummary.RawSummary
	}
	sendChunk(ctx, stream, StreamChunk{Type: "phase_start", Phase: string(PhaseCoding), MaxSteps: MaxStepsPerPhase})
	codeResult, codingMessages, err := o.runPhase(ctx, sessionID, PhaseCoding, CODER_SYSTEM_PROMPT, codingContext, stream, nil)
	if err != nil {
		return err
	}
	sendChunk(ctx, stream, StreamChunk{Type: "phase_complete", Phase: string(PhaseCoding)})

	if o.HasPendingToolConfirm(sessionID) {
		o.mu.Lock()
		o.pendingContinuations[sessionID] = func(ctx context.Context, stream chan<- StreamChunk, phaseOutput string, phaseMessages []provider.LLMMessage) error {
			return o.runReviewingAndFinal(ctx, sessionID, task, plan, planningSummary, phaseOutput, phaseMessages, stream)
		}
		o.mu.Unlock()
		return nil
	}
	return o.runReviewingAndFinal(ctx, sessionID, task, plan, planningSummary, codeResult, codingMessages, stream)
}

// runReviewingAndFinal 执行 REVIEWING 阶段并输出最终总结
func (o *Orchestrator) runReviewingAndFinal(ctx context.Context, sessionID uint, task, plan string,
	planningSummary compression.SessionSummary, codeResult string, codingMessages []provider.LLMMessage, stream chan<- StreamChunk) error {

	codingSummary := o.sessionCompressor.BuildSummary(codingMessages, string(PhaseCoding))
	reviewContext := BuildReviewContext(task, codeResult)
	if codingSummary.RawSummary != "" {
		reviewContext += "\n\n## 执行阶段结构化摘要\n" + codingSummary.RawSummary
	}
	sendChunk(ctx, stream, StreamChunk{Type: "phase_start", Phase: string(PhaseReviewing), MaxSteps: MaxStepsPerPhase})
	reviewResult, _, err := o.runPhase(ctx, sessionID, PhaseReviewing, REVIEWER_SYSTEM_PROMPT, reviewContext, stream, nil)
	if err != nil {
		return err
	}
	sendChunk(ctx, stream, StreamChunk{Type: "phase_complete", Phase: string(PhaseReviewing)})

	if o.HasPendingToolConfirm(sessionID) {
		o.mu.Lock()
		o.pendingContinuations[sessionID] = func(ctx context.Context, stream chan<- StreamChunk, phaseOutput string, phaseMessages []provider.LLMMessage) error {
			return o.finishOrchestration(ctx, sessionID, task, plan, codeResult, phaseOutput, stream)
		}
		o.mu.Unlock()
		return nil
	}
	return o.finishOrchestration(ctx, sessionID, task, plan, codeResult, reviewResult, stream)
}

// finishOrchestration 输出最终总结并结束编排
func (o *Orchestrator) finishOrchestration(ctx context.Context, sessionID uint, task, plan, codeResult, reviewResult string, stream chan<- StreamChunk) error {
	finalSummary := fmt.Sprintf("## 三阶段编排完成\n\n### 执行计划\n%s\n\n### 执行结果\n%s\n\n### 审查结论\n%s",
		truncateStr(plan, 500), truncateStr(codeResult, 500), reviewResult)
	_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, finalSummary, nil)
	sendChunk(ctx, stream, StreamChunk{Type: "message", Content: finalSummary})
	sendChunk(ctx, stream, StreamChunk{Type: "done", Success: true})
	global.LOG.Infof("[Orchestrator] 会话%d: 三阶段编排全部完成", sessionID)
	return nil
}

// HasPendingToolConfirm 当前会话是否有等待确认的工具调用
func (o *Orchestrator) HasPendingToolConfirm(sessionID uint) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.pendingToolConfirms[sessionID] != nil
}

// HasPendingContinuation 当前会话是否有等待继续执行的编排后续步骤
func (o *Orchestrator) HasPendingContinuation(sessionID uint) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.pendingContinuations[sessionID] != nil
}

// ConfirmTool 用户在编排过程中确认/取消某个工具调用后，恢复对应 phase 继续执行
func (o *Orchestrator) ConfirmTool(ctx context.Context, sessionID uint, toolCallID string, confirmed bool, stream chan<- StreamChunk) error {
	o.mu.Lock()
	st := o.pendingToolConfirms[sessionID]
	if st == nil {
		o.mu.Unlock()
		return fmt.Errorf("未找到待确认的工具调用")
	}
	delete(o.pendingToolConfirms, sessionID)
	o.mu.Unlock()

	targetIdx := -1
	var targetTC *provider.ToolCall
	for i := len(st.Messages) - 1; i >= 0; i-- {
		m := st.Messages[i]
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for j := range m.ToolCalls {
			if m.ToolCalls[j].ID == toolCallID {
				targetIdx = i
				targetTC = &m.ToolCalls[j]
				break
			}
		}
		if targetTC != nil {
			break
		}
	}
	if targetTC == nil {
		return fmt.Errorf("tool call not found: %s", toolCallID)
	}

	var resultContent string
	if !confirmed {
		resultContent = "用户取消了此操作"
	} else {
		sendChunk(ctx, stream, StreamChunk{Type: "tool_call", ToolCallID: targetTC.ID, ToolName: targetTC.Function.Name, Content: targetTC.Function.Arguments, Phase: string(st.Phase)})
		execCtx := tools.WithExecOptions(ctx, tools.ExecOptions{
			AllowDangerous: true,
			Timeout:        tools.ExecOptionsFromContext(ctx).Timeout,
		})
		toolResult := o.engine.executor.Execute(execCtx, *targetTC)
		resultContent = o.engine.formatToolResult(toolResult)
		if len(resultContent) > maxToolOutputLength && !strings.Contains(resultContent, "[lazy-ref:") {
			resultContent = resultContent[:maxToolOutputLength] + fmt.Sprintf("\n... [输出过长，已截断。总长度%d 字符]", len(resultContent))
		}
		sendChunk(ctx, stream, StreamChunk{Type: "tool_result", ToolCallID: targetTC.ID, ToolName: targetTC.Function.Name, Content: resultContent, Success: toolResult.Success, Cached: toolResult.Cached, Phase: string(st.Phase)})
	}
	_ = o.engine.sessionMgr.SaveToolResult(sessionID, targetTC.ID, targetTC.Function.Name, resultContent)

	// 重建该 assistant 消息后的 tool 序列：目标用真实结果，其它缺失的调用补占位，保证模型请求合法
	messages := make([]provider.LLMMessage, 0, len(st.Messages)+4)
	messages = append(messages, st.Messages[:targetIdx+1]...)
	messages = append(messages, provider.LLMMessage{Role: "tool", ToolCallID: targetTC.ID, Content: resultContent})
	have := map[string]bool{targetTC.ID: true}
	for _, m := range st.Messages[targetIdx+1:] {
		if m.Role == "tool" && m.ToolCallID != "" {
			have[m.ToolCallID] = true
			messages = append(messages, m)
		}
	}
	for _, tc := range st.Messages[targetIdx].ToolCalls {
		if !have[tc.ID] {
			messages = append(messages, provider.LLMMessage{Role: "tool", ToolCallID: tc.ID, Content: "[操作已中断：用户停止了回复]"})
		}
	}

	phaseOutput, phaseMessages, err := o.runPhaseLoop(ctx, sessionID, st.Phase, st.SystemPrompt, st.HandoffContext, messages, stream, st.State)
	if err != nil {
		return err
	}

	o.mu.Lock()
	cont := o.pendingContinuations[sessionID]
	delete(o.pendingContinuations, sessionID)
	o.mu.Unlock()
	if cont == nil {
		return nil
	}
	return cont(ctx, stream, phaseOutput, phaseMessages)
}

// runPhase 执行单个阶段的 ReAct 循环。
// 返回值：phaseOutput 阶段产出文本，phaseMessages 该阶段累积的消息列表（用于 sessionCompressor 生成摘要），err 错误。
// historyMessages 为该会话的历史消息（用于跨模式/跨阶段共享上下文），可为空。
// runPhase 执行单个阶段的 ReAct 循环（从第 0 步开始）。
// 返回：phaseOutput 阶段产出文本，phaseMessages 该阶段累积的消息列表，err 错误。
// historyMessages 为该会话的历史消息（用于跨模式/跨阶段共享上下文），可为空。
func (o *Orchestrator) runPhase(ctx context.Context, sessionID uint, phase OrchestratorPhase,
	systemPrompt, handoffContext string, stream chan<- StreamChunk, historyMessages []provider.LLMMessage) (phaseOutput string, phaseMessages []provider.LLMMessage, err error) {

	global.LOG.Infof("[Orchestrator] 会话%d: 开始 %s 阶段", sessionID, phase)

	messages := []provider.LLMMessage{
		{Role: "system", Content: systemPrompt},
	}
	// PLANNING 阶段注入历史消息，让规划能承接之前的对话内容
	if phase == PhasePlanning && len(historyMessages) > 0 {
		messages = append(messages, historyMessages...)
	}
	messages = append(messages, provider.LLMMessage{Role: "user", Content: handoffContext})

	return o.runPhaseLoop(ctx, sessionID, phase, systemPrompt, handoffContext, messages, stream, phaseRunState{})
}

// runPhaseLoop 单个 phase 的 ReAct 循环主体；工具确认后由 ConfirmTool 携带保存的状态继续执行
func (o *Orchestrator) runPhaseLoop(ctx context.Context, sessionID uint, phase OrchestratorPhase,
	systemPrompt, handoffContext string, messages []provider.LLMMessage, stream chan<- StreamChunk, state phaseRunState) (phaseOutput string, phaseMessages []provider.LLMMessage, err error) {

	// 构建该阶段的工具定义（白名单过滤）
	allToolDefs := o.engine.registry.ToDefinitions()
	phaseToolDefs := filterToolsByPhase(phase, allToolDefs)

	consecutiveErrors := state.ConsecutiveErrors
	lastCompressionStep := state.LastCompressionStep
	recentCalls := state.RecentCalls

	for step := state.Step; step < MaxStepsPerPhase; step++ {
		select {
		case <-ctx.Done():
			return "", messages, ctx.Err()
		default:
		}

		global.LOG.Infof("[Orchestrator] 会话%d: [%s] 第%d/%d步: 消息数=%d, 错误=%d",
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
			sendChunk(ctx, stream, StreamChunk{
				Type:        "compression_triggered",
				Phase:       string(phase),
				TokensSaved: report.TokensSaved,
				Content:     string(report.Trigger),
				StepNumber:  step + 1,
				MaxSteps:    MaxStepsPerPhase,
			})
		}

		// 调用 LLM（流式）
		resp, err := o.engine.chatStreamWithRetry(ctx, messages, phaseToolDefs, 3, stream, string(phase))
		if err != nil {
			global.LOG.Errorf("[Orchestrator] 会话%d: [%s] LLM调用失败: %v", sessionID, phase, err)
			sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "LLM 调用失败: " + err.Error()})
			return "", messages, err
		}

		hasToolCalls := len(resp.ToolCalls) > 0
		contentLen := len(strings.TrimSpace(resp.Content))
		global.LOG.Infof("[Orchestrator] 会话%d: [%s] LLM返回: 有工具调用=%v, 文本长度=%d",
			sessionID, phase, hasToolCalls, contentLen)

		// 检查阶段完成
		if PhaseComplete(phase, resp) {
			global.LOG.Infof("[Orchestrator] 会话%d: [%s] 阶段完成检测通过", sessionID, phase)
			_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
			if contentLen == 0 {
				return "(阶段完成，无文本输出)", messages, nil
			}
			return resp.Content, messages, nil
		}

		// CODING 阶段必须有显式 "execution completed" 信号才结束
		if phase == PhaseCoding {
			if !hasToolCalls {
				global.LOG.Infof("[Orchestrator] 会话%d: [coding] LLM未调用工具且未输出完成标记，追加引导消息继续执行", sessionID)
				messages = append(messages, provider.LLMMessage{
					Role:    "assistant",
					Content: resp.Content,
				})
				messages = append(messages, provider.LLMMessage{
					Role:    "user",
					Content: "请继续按计划执行任务，使用可用的工具完成操作。如果所有步骤都已执行完成，请输出 \"execution completed\"。",
				})
				_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
				continue
			}
		} else {
			// PLANNING/REVIEWING：无工具调用视为完成
			if !hasToolCalls {
				global.LOG.Infof("[Orchestrator] 会话%d: [%s] 无工具调用，视为阶段完成", sessionID, phase)
				_ = o.engine.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
				if contentLen == 0 {
					return "(阶段完成，无文本输出)", messages, nil
				}
				return resp.Content, messages, nil
			}
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

			sendChunk(ctx, stream, StreamChunk{
				Type:       "tool_call",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    tc.Function.Arguments,
				Phase:      string(phase),
			})

			toolResult := o.engine.executor.Execute(ctx, tc)
			resultContent := o.engine.formatToolResult(toolResult)
			if len(resultContent) > maxToolOutputLength && !strings.Contains(resultContent, "[lazy-ref:") {
				resultContent = resultContent[:maxToolOutputLength] +
					fmt.Sprintf("\n... [输出过长，已截断。总长度%d 字符]", len(resultContent))
			}

			// 确认机制：暂停并保存可恢复状态
			if !toolResult.Success && strings.Contains(toolResult.Error, "confirm required") {
				global.LOG.Infof("[Orchestrator] 会话%d: [%s] 工具%s需要确认", sessionID, phase, tc.Function.Name)
				// 先把已执行的其他工具结果拼上，再保存状态
				messages = append(messages, toolResults...)
				o.mu.Lock()
				o.pendingToolConfirms[sessionID] = &pendingToolConfirm{
					Phase:          phase,
					SystemPrompt:   systemPrompt,
					HandoffContext: handoffContext,
					Messages:       messages,
					State: phaseRunState{
						Step:                step,
						ConsecutiveErrors:   consecutiveErrors,
						LastCompressionStep: lastCompressionStep,
						RecentCalls:         recentCalls,
					},
				}
				o.mu.Unlock()
				sendChunk(ctx, stream, StreamChunk{
					Type:       "confirm_required",
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					Message:    toolResult.Error,
				})
				sendChunk(ctx, stream, StreamChunk{Type: "done", Success: false, Error: "等待用户确认"})
				return "", messages, nil
			}

			sendChunk(ctx, stream, StreamChunk{
				Type:       "tool_result",
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    resultContent,
				Success:    toolResult.Success,
				Cached:     toolResult.Cached,
				Phase:      string(phase),
			})

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

	// 阶段超步数，调用 LLM 生成总结（流式）
	global.LOG.Infof("[Orchestrator] 会话%d: [%s] 阶段达到步数限制，生成总结", sessionID, phase)
	finalResp, err := o.engine.chatStreamWithRetry(ctx, messages, []provider.ToolDefinition{}, 2, stream, string(phase))
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

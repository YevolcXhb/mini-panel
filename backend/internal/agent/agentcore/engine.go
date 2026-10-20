package agentcore

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/contextmgr"
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/repository"
	"github.com/minipanel/minipanel/internal/agent/tools"
	"github.com/minipanel/minipanel/internal/global"
)

// ─── 引擎（参照 deepseek-harness 的 ReactLoopAgent 重写）───
//
// 与旧 engine.go 的关键差异：
//
//  1. **请求是会话的纯函数**。DSH 的注释是 "Every request is derived from
//     the session log."，这里同样：每步开头由 messages 重新组装一次请求，
//     不存在"增量维护一份发送缓冲"的隐式状态。
//
//  2. **压缩是事务**。上下文预算检查、工具结果裁剪、checkpoint 摘要、
//     替换区间在同一个 Compact 调用里完成，且带"必须变小"的不变量。
//     旧实现把压缩散在循环尾部，靠 lastCompressionStep 手工防抖，容易漏。
//
//  3. **溢出自愈**。DSH 把 CONTEXT_WINDOW_EXCEEDED 当成可恢复错误：
//     压缩后重发，而不是把 400 直接抛给用户。这里复刻，最多 2 次。
//
//  4. **错误分类驱动重试**。限流与瞬时故障退避重试；上下文超限压缩重试；
//     其余立即失败。旧实现只认 "429" 字面量。

const (
	// maxOverflowRecoveries 单步内允许的"压缩后重发"次数
	maxOverflowRecoveries = 2
	// maxRetries 限流/瞬时故障的重试次数
	maxRetries = 3
)

// Engine 运维 Agent 引擎。
type Engine struct {
	provider     provider.Provider
	registry     *tools.Registry
	executor     *tools.Executor
	sessionMgr   *repository.SessionManager
	systemPrompt string
	maxSteps     int
	// maxErrors 连续工具失败达到该值即强制收尾
	maxErrors int
	// contextWindow 模型上下文窗口，供 contextmgr 使用
	contextWindow int
	// maxOutputTokens 单次输出上限
	maxOutputTokens int
}

// Options 引擎构造参数。
type Options struct {
	SystemPrompt    string
	MaxSteps        int
	MaxErrors       int
	ContextWindow   int
	MaxOutputTokens int
}

// NewEngine 创建引擎。
func NewEngine(p provider.Provider, registry *tools.Registry, opts Options) *Engine {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 30
	}
	if opts.MaxErrors <= 0 {
		opts.MaxErrors = 5
	}
	if opts.ContextWindow <= 0 {
		opts.ContextWindow = contextmgr.DefaultContextWindow
	}
	if opts.MaxOutputTokens <= 0 {
		opts.MaxOutputTokens = 4096
	}
	return &Engine{
		provider:        p,
		registry:        registry,
		executor:        tools.NewExecutor(registry),
		sessionMgr:      repository.NewSessionManager(),
		systemPrompt:    opts.SystemPrompt,
		maxSteps:        opts.MaxSteps,
		maxErrors:       opts.MaxErrors,
		contextWindow:   opts.ContextWindow,
		maxOutputTokens: opts.MaxOutputTokens,
	}
}

// Run 处理一条用户输入。
func (e *Engine) Run(ctx context.Context, sessionID uint, userInput string, stream chan<- StreamChunk) (err error) {
	defer e.recoverPanic(&err, ctx, stream, "Run")

	messages, err := e.sessionMgr.LoadMessages(sessionID)
	if err != nil {
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "加载会话失败: " + err.Error()})
		return err
	}
	messages = e.ensureSystemPrompt(messages)
	messages = append(messages, provider.LLMMessage{Role: "user", Content: userInput})
	if err := e.sessionMgr.SaveUserMessage(sessionID, userInput); err != nil {
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "保存消息失败: " + err.Error()})
		return err
	}
	return e.loop(ctx, sessionID, messages, stream)
}

// RunRegenerate 删除最后一条 assistant 回复后重新生成。
func (e *Engine) RunRegenerate(ctx context.Context, sessionID uint, stream chan<- StreamChunk) (err error) {
	defer e.recoverPanic(&err, ctx, stream, "RunRegenerate")

	messages, err := e.sessionMgr.LoadMessages(sessionID)
	if err != nil {
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "加载会话失败: " + err.Error()})
		return err
	}
	if len(messages) > 0 && messages[len(messages)-1].Role == "assistant" {
		if err := e.sessionMgr.DeleteLastAssistantMessage(sessionID); err != nil {
			global.LOG.Warnf("[Engine] 会话%d: 删除上一条回复失败: %v", sessionID, err)
		}
		messages = messages[:len(messages)-1]
	}
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "没有可重新生成的上一条回复"})
		return fmt.Errorf("nothing to regenerate")
	}
	messages = e.ensureSystemPrompt(messages)
	return e.loop(ctx, sessionID, messages, stream)
}

// RunWithConfirm 用户确认/拒绝一个待确认工具后继续。
func (e *Engine) RunWithConfirm(ctx context.Context, sessionID uint, toolCallID string, confirmed bool, stream chan<- StreamChunk) (err error) {
	defer e.recoverPanic(&err, ctx, stream, "RunWithConfirm")

	messages, err := e.sessionMgr.LoadMessages(sessionID)
	if err != nil {
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "加载会话失败: " + err.Error()})
		return err
	}

	var targetTC *provider.ToolCall
	targetIdx := -1
	for i := range messages {
		if messages[i].Role != "assistant" {
			continue
		}
		for j := range messages[i].ToolCalls {
			if messages[i].ToolCalls[j].ID == toolCallID {
				targetTC = &messages[i].ToolCalls[j]
				targetIdx = i
				break
			}
		}
		if targetTC != nil {
			break
		}
	}
	if targetTC == nil {
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "找不到待确认的工具调用，请重新发起"})
		return fmt.Errorf("tool call %s not found", toolCallID)
	}

	// 只允许确认"最近一条待确认的工具调用"，防止旧确认把后续对话截断
	lastToolIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) > 0 {
			lastToolIdx = i
			break
		}
	}
	if targetIdx != lastToolIdx {
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "该工具调用已过期，请重新发起操作"})
		return fmt.Errorf("tool call %s is stale", toolCallID)
	}

	var resultContent string
	if confirmed {
		res := e.executor.Execute(ctx, *targetTC)
		resultContent = formatToolResult(res)
	} else {
		resultContent = "用户拒绝了本次操作。请停止该操作并向用户说明当前状态。"
	}
	sendChunk(ctx, stream, StreamChunk{
		Type:       "tool_result",
		ToolCallID: targetTC.ID,
		ToolName:   targetTC.Function.Name,
		Content:    resultContent,
		Success:    confirmed,
	})
	_ = e.sessionMgr.SaveToolResult(sessionID, targetTC.ID, targetTC.Function.Name, resultContent)

	// 重建该 assistant 之后的 tool 序列，把本次确认结果写进去
	prefix := messages[:targetIdx+1]
	rest := messages[targetIdx+1:]
	rebuilt := make([]provider.LLMMessage, 0, len(rest))
	for _, m := range rest {
		if m.Role == "tool" && m.ToolCallID == targetTC.ID {
			m.Content = resultContent
		}
		rebuilt = append(rebuilt, m)
	}
	messages = append(prefix, rebuilt...)
	messages = e.ensureSystemPrompt(messages)
	return e.loop(ctx, sessionID, messages, stream)
}

// ─── 主循环 ───

func (e *Engine) loop(ctx context.Context, sessionID uint, messages []provider.LLMMessage, stream chan<- StreamChunk) error {
	toolDefs := e.registry.ToDefinitions()
	ctxMgr := contextmgr.NewManager(e.provider, e.contextWindow, e.maxOutputTokens, e.maxOutputTokens)
	ctxMgr.Budget.ToolsTokens = estimateToolDefsTokens(toolDefs)

	consecutiveErrors := 0
	concluded := false
	var recentCalls []recentToolCall

	// 运行中插话：用户在 agent 跑动过程中发给本会话的消息先进 inbox，
	// 由本循环在每个步边界取走并注入上下文。
	inbox := DefaultInboxes().Get(sessionID)
	// 每次运行的插话记入数据库与上下文，同时通过 stream 通知前端已接收
	defer inbox.Clear()

	defer func() {
		if !concluded {
			sendChunk(ctx, stream, StreamChunk{
				Type:    "message",
				Content: "处理已结束，请查看上方结果。如需继续，请告诉我。",
			})
			sendChunk(ctx, stream, StreamChunk{Type: "done", Success: true})
		}
	}()

	for step := 1; step <= e.maxSteps; step++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 步边界注入插话：位置在"构造请求之前"，因此刚插进来的话
		// 会立刻体现在本次 LLM 请求里。
		//
		// 必须放在工具批次全部执行完之后 —— 一批 tool_calls 与其结果
		// 必须保持相邻，中间插 user 消息会被 API 拒绝。
		if injected := drainInbox(ctx, sessionID, inbox, stream); len(injected) > 0 {
			for _, text := range injected {
				messages = append(messages, provider.LLMMessage{Role: "user", Content: text})
				_ = e.sessionMgr.SaveUserMessage(sessionID, text)
			}
			global.LOG.Infof("[Engine] 会话%d: 注入 %d 条运行中插话", sessionID, len(injected))
		}

		// 每步开头：预算检查 + 必要时压缩。
		// 这是 DSH 的位置 —— 压缩发生在"构造请求之前"，
		// 而不是"上一轮结束后"，因此溢出恢复天然被覆盖。
		if need, trigger := ctxMgr.ShouldCompact(messages, step, consecutiveErrors); need {
			newMsgs, report, ok := ctxMgr.Compact(ctx, messages, step, trigger, false)
			if ok {
				messages = newMsgs
				sendChunk(ctx, stream, StreamChunk{
					Type:        "compression",
					TokensSaved: report.TokensSaved,
					Content:     report.Trigger,
					Usage:       usagePtr(ctxMgr.Usage(messages)),
				})
			}
		}

		resp, err := e.chatWithRecovery(ctx, sessionID, ctxMgr, &messages, step, toolDefs, stream)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			sendChunk(ctx, stream, StreamChunk{Type: "error", Error: "LLM 调用失败: " + err.Error()})
			return err
		}

		// 无工具调用 = 本轮就是最终答复
		if len(resp.ToolCalls) == 0 {
			content := strings.TrimSpace(resp.Content)
			if content == "" {
				content = "处理完成。如需继续操作，请告诉我。"
				sendChunk(ctx, stream, StreamChunk{Type: "message", Content: content})
			}
			messages = append(messages, provider.LLMMessage{Role: "assistant", Content: content})
			_ = e.sessionMgr.SaveAssistantMessage(sessionID, content, nil)
			concluded = true
			sendChunk(ctx, stream, StreamChunk{Type: "done", Success: true})
			return nil
		}

		messages = append(messages, provider.LLMMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, resp.ToolCalls)

		// 执行工具（DSH 的顺序调度简化版：按模型给出的顺序串行，
		// 运维场景下多数命令有先后依赖，并发带来的风险大于收益）
		for _, tc := range resp.ToolCalls {
			result, skip := e.runOneTool(ctx, sessionID, tc, &recentCalls, &consecutiveErrors, stream)
			messages = append(messages, result)
			if skip {
				// 需要用户确认：已发送 confirm_required，本次运行到此为止
				concluded = true
				sendChunk(ctx, stream, StreamChunk{Type: "done", Success: false, Error: "等待用户确认"})
				return nil
			}
		}

		// 连续失败过多：强制收尾，避免无意义地烧 token
		if consecutiveErrors >= e.maxErrors {
			global.LOG.Warnf("[Engine] 会话%d: 连续%d次工具失败，强制收尾", sessionID, consecutiveErrors)
			messages = append(messages, provider.LLMMessage{
				Role: "user",
				Content: "工具连续多次执行失败。请立即停止调用工具，基于已有信息直接给出结论，" +
					"说明哪些操作失败了、可能的原因是什么、以及建议用户如何处理。不要再调用任何工具。",
			})
			return e.finalSummary(ctx, sessionID, messages, stream, &concluded)
		}

		// 步数将尽：留一步给总结
		if step >= e.maxSteps-1 {
			global.LOG.Warnf("[Engine] 会话%d: 已执行%d步，进入收尾", sessionID, step)
			messages = append(messages, provider.LLMMessage{
				Role:    "user",
				Content: "已经执行了足够多的步骤。请立即停止调用工具，总结刚才完成了什么、当前状态如何，给用户明确答复。",
			})
			return e.finalSummary(ctx, sessionID, messages, stream, &concluded)
		}
	}

	return e.finalSummary(ctx, sessionID, messages, stream, &concluded)
}

// finalSummary 不带工具定义地请求一次收尾总结。
func (e *Engine) finalSummary(ctx context.Context, sessionID uint, messages []provider.LLMMessage, stream chan<- StreamChunk, concluded *bool) error {
	resp, err := e.chatStream(ctx, messages, nil, 1, stream)
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		if err != nil {
			global.LOG.Warnf("[Engine] 会话%d: 收尾总结失败: %v", sessionID, err)
		}
		fallback := "操作已执行，请查看上方工具结果。如需进一步处理，请告诉我。"
		sendChunk(ctx, stream, StreamChunk{Type: "message", Content: fallback})
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, fallback, nil)
	} else {
		_ = e.sessionMgr.SaveAssistantMessage(sessionID, resp.Content, nil)
	}
	*concluded = true
	sendChunk(ctx, stream, StreamChunk{Type: "done", Success: true})
	return nil
}

// runOneTool 执行单个工具并返回可直接追加到 messages 的 tool 消息。
// skip=true 表示流程需中断（等待用户确认）。
func (e *Engine) runOneTool(
	ctx context.Context,
	sessionID uint,
	tc provider.ToolCall,
	recentCalls *[]recentToolCall,
	consecutiveErrors *int,
	stream chan<- StreamChunk,
) (provider.LLMMessage, bool) {
	toolMsg := provider.LLMMessage{Role: "tool", ToolCallID: tc.ID}

	// 重复调用检测：同一工具同一参数连续 3 次触发提醒
	callKey := recentToolCall{Name: tc.Function.Name, Args: tc.Function.Arguments}
	dup := 0
	for _, rc := range *recentCalls {
		if rc.Name == callKey.Name && rc.Args == callKey.Args {
			dup++
		}
	}
	if dup >= 2 {
		warn := fmt.Sprintf("注意：你已经用完全相同的参数连续调用 %s 共 %d 次，结果不会再变。"+
			"请换一种思路，或基于现有结果直接给出结论。", tc.Function.Name, dup+1)
		global.LOG.Warnf("[Engine] 会话%d: 重复调用 %s，已注入提醒", sessionID, tc.Function.Name)
		toolMsg.Content = warn
		*consecutiveErrors++
		_ = e.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, warn)
		return toolMsg, false
	}

	sendChunk(ctx, stream, StreamChunk{
		Type:       "tool_call",
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
		Content:    tc.Function.Arguments,
	})

	res := e.executor.Execute(ctx, tc)
	content := formatToolResult(res)

	// 需要用户确认：中断本轮
	if !res.Success && strings.Contains(res.Error, "confirm required") {
		sendChunk(ctx, stream, StreamChunk{
			Type:       "confirm_required",
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Message:    res.Error,
		})
		// 结果先不落库，等用户确认后由 RunWithConfirm 写入
		toolMsg.Content = content
		return toolMsg, true
	}

	if !res.Success {
		*consecutiveErrors++
	} else {
		*consecutiveErrors = 0
	}

	sendChunk(ctx, stream, StreamChunk{
		Type:       "tool_result",
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
		Content:    content,
		Success:    res.Success,
		Cached:     res.Cached,
	})

	*recentCalls = append(*recentCalls, callKey)
	if len(*recentCalls) > 6 {
		*recentCalls = (*recentCalls)[1:]
	}

	toolMsg.Content = content
	_ = e.sessionMgr.SaveToolResult(sessionID, tc.ID, tc.Function.Name, content)
	return toolMsg, false
}

// ─── LLM 调用与自愈 ───

// chatWithRecovery 带三类恢复的 LLM 调用：
//   - 瞬时故障 / 限流 → 指数退避重试
//   - 上下文超限 → 强制压缩后重发（最多 maxOverflowRecoveries 次）
//   - 其它 → 立即返回错误
func (e *Engine) chatWithRecovery(
	ctx context.Context,
	sessionID uint,
	ctxMgr *contextmgr.Manager,
	messages *[]provider.LLMMessage,
	step int,
	toolDefs []provider.ToolDefinition,
	stream chan<- StreamChunk,
) (*provider.LLMResponse, error) {
	overs := 0
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := sleepBackoff(ctx, attempt); err != nil {
			return nil, err
		}

		resp, err := e.chatStream(ctx, *messages, toolDefs, 0, stream)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		msg := err.Error()

		// 1) 上下文超限：压缩后重发
		if contextmgr.IsOverflowError(msg) {
			if overs >= maxOverflowRecoveries {
				global.LOG.Errorf("[Engine] 会话%d: 上下文超限且压缩已达上限: %v", sessionID, err)
				return nil, err
			}
			overs++
			global.LOG.Warnf("[Engine] 会话%d: 上下文超限，第%d次强制压缩后重发", sessionID, overs)
			newMsgs, report, ok := ctxMgr.CompactForOverflow(ctx, *messages, step)
			if !ok {
				global.LOG.Errorf("[Engine] 会话%d: 强制压缩失败，无法恢复", sessionID)
				return nil, err
			}
			*messages = newMsgs
			sendChunk(ctx, stream, StreamChunk{
				Type:        "compression",
				TokensSaved: report.TokensSaved,
				Content:     "overflow",
				Usage:       usagePtr(ctxMgr.Usage(*messages)),
			})
			attempt-- // 压缩恢复不计入普通重试预算
			continue
		}

		// 2) 限流 / 瞬时故障：重试
		if contextmgr.IsRetryableError(msg) {
			global.LOG.Warnf("[Engine] 会话%d: 可重试错误(第%d次): %v", sessionID, attempt+1, err)
			continue
		}

		// 3) 其它：不重试
		return nil, err
	}
	return nil, lastErr
}

// chatStream 流式调用并按需推送 token。
func (e *Engine) chatStream(ctx context.Context, messages []provider.LLMMessage, toolDefs []provider.ToolDefinition, retries int, stream chan<- StreamChunk) (*provider.LLMResponse, error) {
	streamer, ok := e.provider.(provider.Streamer)
	if !ok {
		return e.provider.Chat(ctx, messages, toolDefs)
	}

	deltaCh, err := streamer.ChatStream(ctx, messages, toolDefs)
	if err != nil {
		return nil, err
	}

	resp := &provider.LLMResponse{}
	var toolCalls []provider.ToolCall
	pending := map[int]*provider.ToolCall{}

	for delta := range deltaCh {
		switch delta.Type {
		case "content":
			resp.Content += delta.Content
			sendChunk(ctx, stream, StreamChunk{Type: "token", Content: delta.Content})
		case "tool_call":
			// 按 index 聚合：流式工具调用的 name 只在首片出现，
			// arguments 是逐片增量拼接，必须累积而不是覆盖。
			idx := delta.ToolCallIndex
			tc, exists := pending[idx]
			if !exists {
				tc = &provider.ToolCall{ID: delta.ToolCallID, Type: delta.ToolCallType}
				pending[idx] = tc
			}
			if delta.ToolCallID != "" && tc.ID == "" {
				tc.ID = delta.ToolCallID
			}
			if delta.FunctionName != "" {
				tc.Function.Name = delta.FunctionName
			}
			tc.Function.Arguments += delta.ArgumentsDelta
		case "usage":
			if delta.Usage != nil {
				resp.Usage = *delta.Usage
			}
		case "error":
			return nil, fmt.Errorf("stream error: %s", delta.Content)
		case "done":
			// 正常结束
		}
	}

	// 按 index 顺序还原
	maxIdx := -1
	for idx := range pending {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	for i := 0; i <= maxIdx; i++ {
		if tc, ok := pending[i]; ok {
			toolCalls = append(toolCalls, *tc)
		}
	}
	resp.ToolCalls = toolCalls
	return resp, nil
}

func sleepBackoff(ctx context.Context, attempt int) error {
	if attempt == 0 {
		return nil
	}
	wait := time.Duration(int(math.Pow(2, float64(attempt-1))) * 1000 * 1000 * 1000)
	if wait > 8*time.Second {
		wait = 8 * time.Second
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

// ─── 辅助 ───

type recentToolCall struct {
	Name string
	Args string
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

func (e *Engine) recoverPanic(err *error, ctx context.Context, stream chan<- StreamChunk, where string) {
	if r := recover(); r != nil {
		global.LOG.Errorf("[Engine] %s panic: %v", where, r)
		*err = fmt.Errorf("engine panic: %v", r)
		sendChunk(ctx, stream, StreamChunk{Type: "error", Error: fmt.Sprintf("引擎内部错误: %v", r)})
		sendChunk(ctx, stream, StreamChunk{Type: "done", Success: false})
	}
}

func formatToolResult(res tools.ToolResult) string {
	if res.Success {
		if res.Result == "" {
			return "执行成功（无输出）"
		}
		return res.Result
	}
	if res.Result != "" {
		return fmt.Sprintf("执行失败: %s\n输出:\n%s", res.Error, res.Result)
	}
	return fmt.Sprintf("执行失败: %s", res.Error)
}

func estimateToolDefsTokens(defs []provider.ToolDefinition) int {
	total := 0
	for _, d := range defs {
		total += 8
		total += contextmgr.EstimateText(d.Function.Name)
		total += contextmgr.EstimateText(d.Function.Description)
		// 参数 schema 序列化后通常比字面更长，乘个系数近似
		total += contextmgr.EstimateText(fmt.Sprintf("%v", d.Function.Parameters))
	}
	return total
}

func usagePtr(u contextmgr.Usage) *StreamUsage {
	return &StreamUsage{
		Tokens:        u.Tokens,
		Threshold:     u.Threshold,
		ContextWindow: u.ContextWindow,
		Percent:       u.Percent,
		Compactions:   u.Compactions,
	}
}

func sendChunk(ctx context.Context, stream chan<- StreamChunk, chunk StreamChunk) {
	select {
	case <-ctx.Done():
	case stream <- chunk:
	}
}

// drainInbox 取走积压插话并向前端广播"已接收"事件。
//
// 前端收到 user_input_received 后可以把那条临时气泡从"发送中"改成"已送达"，
// 用户就知道话已经进了 agent 的上下文，而不是被丢弃。
func drainInbox(ctx context.Context, sessionID uint, inbox *Inbox, stream chan<- StreamChunk) []string {
	if inbox == nil {
		return nil
	}
	msgs := inbox.Drain()
	if len(msgs) == 0 {
		return nil
	}
	for _, text := range msgs {
		sendChunk(ctx, stream, StreamChunk{
			Type:    "user_input_received",
			Content: text,
			Message: "插话已注入上下文",
		})
	}
	return msgs
}

// BuildTitleFromContent 从首条消息生成会话标题。
func BuildTitleFromContent(content string) string {
	s := strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	r := []rune(s)
	if len(r) <= 20 {
		return s
	}
	return string(r[:20]) + "..."
}

package contextmgr

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
)

// ─── 压缩事务（对应 DSH 的 compactSurfaceRegion）───
//
// DSH 的压缩是一条有始有终的事务：先落一个 compaction/start 标记作为锁，
// 摘要产出后替换掉被压缩区间、写入 checkpoint 消息，最后落 end 标记。
// 会话日志因此始终可回放，且"压缩中"这个状态是可检测的。
//
// 这里是内存态版本，但保留三条最关键的不变量：
//  1. system 提示词永远在首位且不被压缩
//  2. 压缩区间不切断 tool_call / tool_result 原子对
//  3. 压缩后必须真正变小，否则放弃这次压缩

// Manager 管理单次对话的上下文预算与压缩。
type Manager struct {
	Budget     Budget
	Summarizer *Summarizer
	// stepInterval 两次压缩之间至少间隔的步数，避免每步都触发摘要调用
	stepInterval int
	// lastCompactionStep 上次压缩发生时的步号
	lastCompactionStep int
	// compactionCount 本次会话已压缩次数
	compactionCount int
	// maxCompactions 单次运行允许的压缩上限，防止摘要调用死循环
	maxCompactions int
}

// NewManager 创建上下文管理器。
//
// 参数语义（顺序容易搞错，这里写清楚）：
//   - contextWindow：模型上下文窗口，决定压缩水位，百万级窗口也要能撑住
//   - maxOutputTokens：单次回复输出上限，会从消息预算里扣掉
//   - maxTokens：交给 Summarizer 做摘要调用时的输出上限
func NewManager(p provider.Provider, contextWindow, maxOutputTokens, maxTokens int) *Manager {
	return &Manager{
		Budget:             NewBudget(contextWindow, maxOutputTokens),
		Summarizer:         NewSummarizer(p, maxTokens),
		stepInterval:       2,
		maxCompactions:     8,
		lastCompactionStep: -99,
	}
}

// Report 一次压缩的结果。
type Report struct {
	Applied        bool   `json:"applied"`
	Trigger        string `json:"trigger"`
	MessagesBefore int    `json:"messages_before"`
	MessagesAfter  int    `json:"messages_after"`
	TokensBefore   int    `json:"tokens_before"`
	TokensAfter    int    `json:"tokens_after"`
	TokensSaved    int    `json:"tokens_saved"`
	PrunedResults  int    `json:"pruned_results"`
	Summarized     bool   `json:"summarized"`
	SummaryPreview string `json:"summary_preview"`
}

// ShouldCompact 判定本轮结束后是否要压缩。
//
// 触发条件按优先级：
//  1. token 超过水位 —— 硬指标，必须压
//  2. 连续错误过多 —— 上下文里堆满失败输出，压一次能让模型换个思路
//  3. 步数间隔到了且 token 已达半水位 —— 预防性压缩，摊平后续增长
func (m *Manager) ShouldCompact(messages []provider.LLMMessage, step, consecutiveErrors int) (bool, string) {
	tokens := EstimateMessages(messages) + m.Budget.ToolsTokens
	threshold := m.Budget.Threshold()
	if tokens >= threshold {
		return true, "token"
	}
	if step-m.lastCompactionStep < m.stepInterval {
		return false, ""
	}
	if consecutiveErrors >= 3 && tokens >= threshold/3 {
		return true, "errors"
	}
	if step-m.lastCompactionStep >= 4 && tokens >= threshold/2 {
		return true, "steps"
	}
	return false, ""
}

// Compact 执行一次压缩，返回新消息列表与报告。
//
// 返回的 ok=false 表示"不需要压"或"压了没变小"，调用方应继续用原消息。
func (m *Manager) Compact(
	ctx context.Context,
	messages []provider.LLMMessage,
	step int,
	trigger string,
	force bool,
) ([]provider.LLMMessage, Report, bool) {
	report := Report{
		Trigger:        trigger,
		MessagesBefore: len(messages),
		TokensBefore:   EstimateMessages(messages) + m.Budget.ToolsTokens,
	}

	if len(messages) < 3 {
		report.MessagesAfter = len(messages)
		report.TokensAfter = report.TokensBefore
		return messages, report, false
	}
	if m.compactionCount >= m.maxCompactions {
		global.LOG.Warnf("[ContextMgr] 已达到单次运行压缩上限(%d)，跳过", m.maxCompactions)
		report.MessagesAfter = len(messages)
		report.TokensAfter = report.TokensBefore
		return messages, report, false
	}

	// 第一步：确定性裁剪工具结果。不需要模型参与，
	// 往往一次裁剪就够把水位降到阈值之下，省掉一次摘要调用。
	pruned, prunedCount := PruneMessages(messages)
	report.PrunedResults = prunedCount
	prunedTokens := EstimateMessages(pruned) + m.Budget.ToolsTokens
	if !force && prunedCount > 0 && prunedTokens < m.Budget.Threshold() {
		report.Applied = true
		report.MessagesAfter = len(pruned)
		report.TokensAfter = prunedTokens
		report.TokensSaved = report.TokensBefore - prunedTokens
		m.lastCompactionStep = step
		m.compactionCount++
		global.LOG.Infof("[ContextMgr] 仅裁剪工具结果即达标: %d 条被裁剪, tokens %d→%d",
			prunedCount, report.TokensBefore, prunedTokens)
		return pruned, report, true
	}
	messages = pruned
	report.TokensBefore = prunedTokens

	// 第二步：定位保留的尾段起点
	retain := m.Budget.RetainTokens()
	if force {
		// 强制压缩（溢出恢复）时尽量多压，只留最小尾段
		retain = MinRetainTokens
	}
	start := SelectTailStart(messages, retain)
	if start <= 1 || start >= len(messages) {
		report.MessagesAfter = len(messages)
		report.TokensAfter = report.TokensBefore
		return messages, report, false
	}

	head := messages[:1] // system（若首条非 system 也保留，作为锚点）
	span := messages[1:start]
	tail := messages[start:]
	if len(span) == 0 {
		report.MessagesAfter = len(messages)
		report.TokensAfter = report.TokensBefore
		return messages, report, false
	}

	// 第三步：让模型产出 checkpoint；失败则用确定性摘要
	body, err := m.Summarizer.Summarize(ctx, append(append([]provider.LLMMessage{}, head...), span...))
	if err == nil {
		report.Summarized = true
	}
	report.SummaryPreview = oneLine(body, 120)

	// 第四步：组装压缩后的上下文
	compressed := make([]provider.LLMMessage, 0, 2+len(tail))
	compressed = append(compressed, head...)
	compressed = append(compressed, provider.LLMMessage{
		Role:    "user",
		Content: FormatCheckpoint(body),
	})
	compressed = append(compressed, tail...)

	afterTokens := EstimateMessages(compressed) + m.Budget.ToolsTokens
	// 不变量：压缩必须真正变小，否则放弃。
	// 否则会出现"压缩一次反而更长"，越压越差，甚至反复触发。
	if afterTokens >= report.TokensBefore {
		global.LOG.Warnf("[ContextMgr] 压缩后未变小(%d→%d)，放弃本次压缩",
			report.TokensBefore, afterTokens)
		report.MessagesAfter = len(messages)
		report.TokensAfter = report.TokensBefore
		return messages, report, false
	}

	report.Applied = true
	report.MessagesAfter = len(compressed)
	report.TokensAfter = afterTokens
	report.TokensSaved = report.TokensBefore - afterTokens
	m.lastCompactionStep = step
	m.compactionCount++

	global.LOG.Infof("[ContextMgr] 压缩完成: trigger=%s summarized=%v %d→%d 条, tokens %d→%d (省 %d, %.0f%%), 裁剪工具结果 %d 条",
		trigger, report.Summarized,
		report.MessagesBefore, report.MessagesAfter,
		report.TokensBefore, afterTokens, report.TokensSaved,
		float64(report.TokensSaved)*100/float64(maxInt(report.TokensBefore, 1)),
		prunedCount)
	return compressed, report, true
}

// CompactForOverflow 处理"上下文超限"的硬失败：强制压到最小可用形态后重试。
func (m *Manager) CompactForOverflow(ctx context.Context, messages []provider.LLMMessage, step int) ([]provider.LLMMessage, Report, bool) {
	return m.Compact(ctx, messages, step, "overflow", true)
}

// Usage 返回会话的上下文使用情况，供前端展示。
type Usage struct {
	Tokens        int     `json:"tokens"`
	Threshold     int     `json:"threshold"`
	ContextWindow int     `json:"context_window"`
	Percent       float64 `json:"percent"`
	Compactions   int     `json:"compactions"`
}

// Usage 当前上下文的占用情况。
func (m *Manager) Usage(messages []provider.LLMMessage) Usage {
	tokens := EstimateMessages(messages) + m.Budget.ToolsTokens
	threshold := m.Budget.Threshold()
	window := m.Budget.ContextWindow
	percent := 0.0
	if window > 0 {
		percent = float64(tokens) * 100 / float64(window)
	}
	return Usage{
		Tokens:        tokens,
		Threshold:     threshold,
		ContextWindow: window,
		Percent:       percent,
		Compactions:   m.compactionCount,
	}
}

// ─── 错误分类 ───
//
// DSH 用结构化错误码 CONTEXT_WINDOW_EXCEEDED_CODE 判断；
// Go 这边各 provider 的报错文本不一致，只能靠特征串匹配。
// 覆盖 DeepSeek / OpenAI / Anthropic / Ollama / vLLM 的常见措辞。
var overflowSignatures = []string{
	"context_length_exceeded",
	"context length exceeded",
	"maximum context length",
	"reduce the length of the messages",
	"prompt is too long",
	"input is too long",
	"too many tokens",
	"exceeds the maximum number of tokens",
	"exceed context limit",
	"context window",
	"request too large",
	"string too long",
}

// IsOverflowError 判断错误是否为"上下文超限"这类可自愈的失败。
func IsOverflowError(msg string) bool {
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	for _, sig := range overflowSignatures {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// IsRetryableError 判断是否属于限流/瞬时故障，值得退避重试。
// 上下文超限不算 —— 那种必须压缩后重发，退避没有意义。
func IsRetryableError(msg string) bool {
	if IsOverflowError(msg) {
		return false
	}
	lower := strings.ToLower(msg)
	for _, sig := range []string{
		"429", "rate limit", "too many requests",
		"500", "502", "503", "504",
		"timeout", "timed out", "connection reset",
		"temporarily unavailable", "overloaded",
		"eof", "broken pipe",
	} {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// String 让 Report 能直接进日志。
func (r Report) String() string {
	return fmt.Sprintf("trigger=%s applied=%v summarized=%v msgs %d→%d tokens %d→%d saved=%d pruned=%d",
		r.Trigger, r.Applied, r.Summarized, r.MessagesBefore, r.MessagesAfter,
		r.TokensBefore, r.TokensAfter, r.TokensSaved, r.PrunedResults)
}

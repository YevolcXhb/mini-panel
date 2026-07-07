package compression

import (
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
)

// ContextCompressor 压缩策略接口
type ContextCompressor interface {
	// ShouldCompress 在每次 ReAct 迭代评估是否需要压缩
	ShouldCompress(ctx CompressionContext) bool
	// Compress 执行压缩，返回新消息列表和诊断报告
	// 契约：不得切断 tool_call/tool_result 原子对；必须保留 system prompt 在首位
	Compress(messages []provider.LLMMessage, ctx CompressionContext) ([]provider.LLMMessage, CompressionReport)
}

// ─── Micro-compression（循环内安全网）───

// MicroCompressionStrategy 在单个 ReAct 循环内频繁、窄口径的压缩。
// 双触发模型：SEMANTIC（语义边界）∨ FORCED（步数/错误阈值）
type MicroCompressionStrategy struct {
	StepInterval        int // 强制触发：距上次压缩的步数间隔
	MaxErrors           int // 强制触发：连续错误阈值
	MinHead             int // 始终保留的头部消息数（system prompt）
	TailTarget          int // 工作集保留的尾部消息数
	LargeOutputThreshold int // 超过此长度则注册 lazy-ref
	SemanticKeywords    map[string]bool
}

// NewMicroCompressionStrategy 创建默认配置的微压缩策略
func NewMicroCompressionStrategy() *MicroCompressionStrategy {
	return &MicroCompressionStrategy{
		StepInterval:         10,
		MaxErrors:            3,
		MinHead:              1,
		TailTarget:           15,
		LargeOutputThreshold: 1024,
		SemanticKeywords: map[string]bool{
			"step completed":      true,
			"moving on":           true,
			"next step":           true,
			"summarize":           true,
			"let me summarize":    true,
			"here is a summary":   true,
			"overview of what":    true,
			"步骤完成":               true,
			"继续下一步":              true,
			"总结一下":               true,
		},
	}
}

// hasSemanticTrigger 检查最近助手消息是否含语义边界关键词
func (m *MicroCompressionStrategy) hasSemanticTrigger(ctx CompressionContext) bool {
	if ctx.LastMessage == "" {
		return false
	}
	lower := strings.ToLower(ctx.LastMessage)
	for kw := range m.SemanticKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// detectTrigger 判定触发类型（语义优先于强制）
func (m *MicroCompressionStrategy) detectTrigger(ctx CompressionContext) CompressionTrigger {
	if m.hasSemanticTrigger(ctx) {
		return TriggerSemantic
	}
	return TriggerForced
}

// ShouldCompress 双触发逻辑
func (m *MicroCompressionStrategy) ShouldCompress(ctx CompressionContext) bool {
	hasSemantic := m.hasSemanticTrigger(ctx)
	hasForced := ctx.StepNumber-ctx.LastCompressionStep >= m.StepInterval ||
		ctx.ConsecutiveErrors >= m.MaxErrors
	return hasSemantic || hasForced
}

// Compress 执行微压缩
func (m *MicroCompressionStrategy) Compress(messages []provider.LLMMessage, ctx CompressionContext) ([]provider.LLMMessage, CompressionReport) {
	if len(messages) <= m.MinHead+m.TailTarget {
		return messages, CompressionReport{Trigger: m.detectTrigger(ctx), StrategyName: "micro_compression"}
	}

	// 1. 找到原子对安全切点
	safeCut := FindSafeCut(messages, m.TailTarget, m.MinHead)
	adjusted := safeCut != len(messages)-m.TailTarget

	head := messages[:m.MinHead]
	compressible := messages[m.MinHead:safeCut]
	tail := messages[safeCut:]

	// 2. 构建 ToolCallID→ToolName 映射（用于标注摘要中的工具名）
	toolNames := buildToolNameMap(messages)

	// 3. 遍历 compressible 构建摘要
	var summaryParts []string
	var lazyRefs []LazyRef

	for _, msg := range compressible {
		if msg.Role == "tool" {
			name := toolNames[msg.ToolCallID]
			if name == "" {
				name = "unknown"
			}
			label := "✓"
			if isFailedToolResult(msg.Content) {
				label = "✗"
			}
			detail := buildToolDetail(msg.Content, m.LargeOutputThreshold, &lazyRefs)
			if detail != "" {
				summaryParts = append(summaryParts, fmt.Sprintf("%s %s: %s", label, name, detail))
			}
		} else if msg.Role == "assistant" && len(msg.Content) > 20 {
			lower := strings.ToLower(msg.Content)
			if containsAny(lower, "plan", "approach", "strategy", "fix", "change", "implement", "计划", "方案", "修复", "修改") {
				snippet := msg.Content
				if len(snippet) > 200 {
					snippet = snippet[:200]
				}
				summaryParts = append(summaryParts, "→ "+snippet)
			}
		}
	}

	summaryText := "(见最近消息获取上下文)"
	if len(summaryParts) > 0 {
		summaryText = strings.Join(summaryParts, "\n")
	}

	// 4. 附加 lazy-ref 引用列表
	if len(lazyRefs) > 0 {
		var refLines []string
		for _, ref := range lazyRefs {
			r := string(ref)
			if len(r) > 24 {
				r = r[:24]
			}
			refLines = append(refLines, fmt.Sprintf("  - %s...  (hashed)", r))
		}
		summaryText += "\n\n**Lazy-loaded references (re-fetch on demand via resolve_lazy_ref tool):**\n" + strings.Join(refLines, "\n")
	}

	// 5. 组装压缩后消息
	compressed := make([]provider.LLMMessage, 0, m.MinHead+1+len(tail))
	compressed = append(compressed, head[0]) // system prompt
	compressed = append(compressed, provider.LLMMessage{
		Role: "user",
		Content: fmt.Sprintf(
			"[Micro-Compression — before step %d]:\n%s\n\n"+
				"The above is a compressed summary of earlier steps. Continue working on the task.",
			ctx.StepNumber, summaryText),
	})
	compressed = append(compressed, tail...)

	report := CompressionReport{
		Trigger:            m.detectTrigger(ctx),
		TokensSaved:        EstimateTokensSaved(compressible),
		MessagesCompressed: len(compressible),
		StrategyName:       "micro_compression",
		SafeCutAdjusted:    adjusted,
	}

	global.LOG.Infof("[Compression] 微压缩触发: trigger=%s, tokens_saved=%d, messages_compressed=%d, safe_cut_adjusted=%v, step=%d",
		report.Trigger, report.TokensSaved, report.MessagesCompressed, report.SafeCutAdjusted, ctx.StepNumber)

	return compressed, report
}

// ─── Session compression（阶段交接）───

// SessionCompressionStrategy 在阶段边界（Planner→Coder、Coder→Reviewer）生成结构化摘要。
// 摘要替换整个 raw-message 历史，作为下一阶段的根上下文。
type SessionCompressionStrategy struct {
	DecisionSignals map[string]bool
	TrialSignals    map[string]bool
}

// NewSessionCompressionStrategy 创建阶段交接压缩策略
func NewSessionCompressionStrategy() *SessionCompressionStrategy {
	return &SessionCompressionStrategy{
		DecisionSignals: map[string]bool{
			"decision": true, "chose": true, "using": true, "adopt": true,
			"pattern": true, "architecture": true, "configuration": true,
			"决定": true, "采用": true, "配置": true,
		},
		TrialSignals: map[string]bool{
			"tried": true, "attempted": true, "didn't work": true,
			"failed": true, "error": true, "reverting": true,
			"尝试": true, "失败": true, "回滚": true,
		},
	}
}

// ShouldCompress 阶段交接时总是返回 true（由编排器在边界调用）
func (s *SessionCompressionStrategy) ShouldCompress(ctx CompressionContext) bool {
	return true
}

// BuildSummary 构建结构化摘要（不压缩，仅提取）
func (s *SessionCompressionStrategy) BuildSummary(messages []provider.LLMMessage, phaseName string) SessionSummary {
	summary := SessionSummary{Phase: phaseName}
	toolNames := buildToolNameMap(messages)

	for _, msg := range messages {
		if msg.Role == "tool" {
			name := toolNames[msg.ToolCallID]
			if name == "" {
				name = "unknown"
			}
			if !isFailedToolResult(msg.Content) && len(msg.Content) > 80 {
				detail := ScrubSensitiveData(msg.Content)
				if len(detail) > 150 {
					detail = detail[:150]
				}
				summary.KeyAchievements = append(summary.KeyAchievements, fmt.Sprintf("%s: %s", name, detail))
			} else if isFailedToolResult(msg.Content) {
				detail := ScrubSensitiveData(msg.Content)
				if len(detail) > 150 {
					detail = detail[:150]
				}
				summary.TrialPaths = append(summary.TrialPaths, fmt.Sprintf("%s error: %s", name, detail))
			}
		} else if msg.Role == "assistant" && msg.Content != "" {
			lower := strings.ToLower(msg.Content)
			if containsAny(lower, s.decisionSignalSlice()...) {
				snippet := msg.Content
				if len(snippet) > 200 {
					snippet = snippet[:200]
				}
				summary.DesignDecisions = append(summary.DesignDecisions, snippet)
			}
			if containsAny(lower, s.trialSignalSlice()...) {
				snippet := msg.Content
				if len(snippet) > 200 {
					snippet = snippet[:200]
				}
				summary.TrialPaths = append(summary.TrialPaths, snippet)
			}
		}
	}

	// 去重
	summary.KeyAchievements = dedup(summary.KeyAchievements)
	summary.DesignDecisions = dedup(summary.DesignDecisions)
	summary.TrialPaths = dedup(summary.TrialPaths)

	// 构建文本
	summary.RawSummary = s.renderSummary(summary)
	return summary
}

// Compress 执行阶段交接压缩
func (s *SessionCompressionStrategy) Compress(messages []provider.LLMMessage, ctx CompressionContext) ([]provider.LLMMessage, CompressionReport) {
	summary := s.BuildSummary(messages, ctx.PhaseName)

	var systemPrompt provider.LLMMessage
	if len(messages) > 0 && messages[0].Role == "system" {
		systemPrompt = messages[0]
	} else {
		systemPrompt = provider.LLMMessage{Role: "system", Content: ""}
	}

	compressed := []provider.LLMMessage{
		systemPrompt,
		{
			Role: "user",
			Content: fmt.Sprintf(
				"[Session Handoff — %s phase completed]\n\n%s\n\n"+
					"The above summarises the previous phase's work. Continue with your role's objective.",
				ctx.PhaseName, summary.RawSummary),
		},
	}

	report := CompressionReport{
		Trigger:            TriggerPhaseTransition,
		TokensSaved:        EstimateTokensSaved(messages[1:]),
		MessagesCompressed: len(messages) - 1,
		StrategyName:       "session_compression",
		SafeCutAdjusted:    false,
	}

	global.LOG.Infof("[Compression] 会话压缩触发: phase=%s, tokens_saved=%d, messages_compressed=%d, achievements=%d, decisions=%d, trials=%d",
		ctx.PhaseName, report.TokensSaved, report.MessagesCompressed,
		len(summary.KeyAchievements), len(summary.DesignDecisions), len(summary.TrialPaths))

	return compressed, report
}

func (s *SessionCompressionStrategy) renderSummary(summary SessionSummary) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("## %s Phase Summary", titleCase(summary.Phase)))
	if len(summary.KeyAchievements) > 0 {
		parts = append(parts, "### Key Achievements")
		for _, a := range summary.KeyAchievements[:min(5, len(summary.KeyAchievements))] {
			parts = append(parts, "- "+a)
		}
	}
	if len(summary.RemainingIssues) > 0 {
		parts = append(parts, "### Remaining Issues")
		for _, i := range summary.RemainingIssues[:min(3, len(summary.RemainingIssues))] {
			parts = append(parts, "- "+i)
		}
	}
	if len(summary.DesignDecisions) > 0 {
		parts = append(parts, "### Design Decisions")
		for _, d := range summary.DesignDecisions[:min(3, len(summary.DesignDecisions))] {
			parts = append(parts, "- "+d)
		}
	}
	if len(summary.TrialPaths) > 0 {
		parts = append(parts, "### Trial Paths (avoided)")
		for _, t := range summary.TrialPaths[:min(3, len(summary.TrialPaths))] {
			parts = append(parts, "- "+t)
		}
	}
	return strings.Join(parts, "\n")
}

func (s *SessionCompressionStrategy) decisionSignalSlice() []string {
	out := make([]string, 0, len(s.DecisionSignals))
	for k := range s.DecisionSignals {
		out = append(out, k)
	}
	return out
}

func (s *SessionCompressionStrategy) trialSignalSlice() []string {
	out := make([]string, 0, len(s.TrialSignals))
	for k := range s.TrialSignals {
		out = append(out, k)
	}
	return out
}

// ─── 辅助函数 ───

// buildToolNameMap 从消息列表构建 ToolCallID→ToolName 映射
func buildToolNameMap(messages []provider.LLMMessage) map[string]string {
	names := make(map[string]string)
	for _, m := range messages {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				names[tc.ID] = tc.Function.Name
			}
		}
	}
	return names
}

// isFailedToolResult 判断 tool 消息内容是否表示失败
func isFailedToolResult(content string) bool {
	return strings.HasPrefix(content, "执行失败") || strings.HasPrefix(content, "Error:") || strings.HasPrefix(content, "FAILED")
}

// buildToolDetail 构建工具结果摘要（含 lazy-ref 注册）
func buildToolDetail(content string, threshold int, lazyRefs *[]LazyRef) string {
	if content == "" {
		return ""
	}
	if len(content) > threshold {
		ref := RegisterLazyRef(content)
		*lazyRefs = append(*lazyRefs, LazyRef(ref))
		preview := ScrubSensitiveData(content)
		if len(preview) > 80 {
			preview = preview[:80]
		}
		return fmt.Sprintf("[lazy-ref:%s] %s...", ref, preview)
	}
	detail := ScrubSensitiveData(content)
	if len(detail) > 120 {
		detail = detail[:120]
	}
	return detail
}

// containsAny 检查 s 是否包含任意子串
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// dedup 保序去重（按前80字符）
func dedup(items []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		key := item
		if len(key) > 80 {
			key = key[:80]
		}
		if !seen[key] {
			seen[key] = true
			result = append(result, item)
		}
	}
	return result
}

// titleCase 首字母大写
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// min 返回较小值（Go 1.21+ 已内置 min，此处兼容旧版本）
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

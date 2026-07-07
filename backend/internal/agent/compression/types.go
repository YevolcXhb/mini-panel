package compression

import (
	"github.com/minipanel/minipanel/internal/agent/provider"
)

// CompressionTrigger 压缩触发原因
type CompressionTrigger string

const (
	TriggerSemantic        CompressionTrigger = "semantic"         // 模型语义边界（如 "step completed"）
	TriggerForced          CompressionTrigger = "forced"           // 安全阈值（步数/错误计数）
	TriggerPhaseTransition CompressionTrigger = "phase_transition" // 阶段交接
	TriggerManual          CompressionTrigger = "manual"           // 显式调用
)

// CompressionContext 压缩决策上下文（每次 ReAct 迭代传入）
type CompressionContext struct {
	StepNumber          int    // 当前步数
	MessageCount        int    // 当前消息总数
	ConsecutiveErrors   int    // 连续错误数
	PhaseName           string // 当前阶段名
	LastMessage         string // 最近一条助手消息内容
	LastCompressionStep int    // 上次压缩发生的步数
}

// CompressionReport 压缩诊断报告
type CompressionReport struct {
	Trigger            CompressionTrigger
	TokensSaved        int    // 估算节省的 token 数
	MessagesCompressed int    // 被压缩的消息数
	StrategyName       string // 策略名
	SafeCutAdjusted    bool   // 切点是否为保护原子对而调整
}

// SessionSummary 阶段交接时的结构化摘要
type SessionSummary struct {
	Phase           string
	KeyAchievements []string
	RemainingIssues []string
	DesignDecisions []string
	TrialPaths      []string
	RawSummary      string
}

// LazyRef 是大输出的内容哈希引用，可按需取回
type LazyRef string

// FindSafeCut 向后遍历找到不切断 tool_call/tool_result 原子对的切点。
// 返回 tail 的起始索引（inclusive），保证不落在 tool_result 或 assistant(含 tool_calls) 消息上。
// 最小返回 minHead，避免切掉 system prompt。
func FindSafeCut(messages []provider.LLMMessage, tailTarget, minHead int) int {
	cut := len(messages) - tailTarget
	for cut > minHead {
		m := messages[cut]
		// 跳过 tool 结果消息（必须与其 tool_call 配对）
		if m.Role == "tool" {
			cut--
			continue
		}
		// 跳过带 tool_calls 的 assistant 消息（必须与其 tool 结果配对）
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			cut--
			continue
		}
		break
	}
	if cut < minHead {
		return minHead
	}
	return cut
}

// EstimateTokensSaved 粗略估算节省的 token 数（1 token ≈ 4 字符）
func EstimateTokensSaved(messages []provider.LLMMessage) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += len(m.Content)
	}
	return totalChars / 4
}

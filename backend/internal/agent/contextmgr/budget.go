package contextmgr

import (
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// ─── 上下文预算（对应 DSH 的 compaction-basic 压力/保留量推导）───
//
// DSH 的判定方式值得照搬：压缩触发点由"模型上下文窗口 × 比例"和
// "窗口 - 输出预留 - 余量"两者取小，保留尾部再按"未被压缩的原始尾段
// 累计 token 达到保留量"来定位切点 —— 而不是按消息条数。
//
// 按条数的问题是：一条 5000 字的命令输出和一条 10 字的确认，
// 条数权重一样，但 token 差 500 倍。按 token 累计才能稳定控制水位。

const (
	// DefaultContextWindow 未知模型时的假定上下文窗口（主流模型普遍 ≥128K，
	// 这里保守取 64K，宁可早压缩也不要撞 400）
	DefaultContextWindow = 65536

	// DefaultThresholdRatio 请求压力达到窗口的该比例即触发压缩
	DefaultThresholdRatio = 0.80

	// DefaultHeadroomTokens 压缩后仍需留出的输出与安全余量
	DefaultHeadroomTokens = 8192

	// DefaultRetainRatio 压缩时按 token 保留的原始尾段比例
	DefaultRetainRatio = 0.20

	// MinRetainTokens 尾段保留量下限，低于此值模型会丢失当前工作现场
	MinRetainTokens = 2048
)

// Budget 一次压缩会话的预算配置。
type Budget struct {
	// ContextWindow 模型上下文窗口（token）
	ContextWindow int
	// ThresholdRatio 触发压缩的水位比例
	ThresholdRatio float64
	// HeadroomTokens 输出与管理开销预留
	HeadroomTokens int
	// RetainRatio 压缩后保留原始尾段的比例（相对可用消息预算）
	RetainRatio float64
	// MaxTokens 单次请求的输出上限（也会算进窗口占用）
	MaxTokens int
	// ToolsTokens 每轮固定随请求发送的工具 schema 占用
	ToolsTokens int
}

// NewBudget 构造预算。
//
// contextWindow 是模型实际窗口（来自用户配置）。它为 0 或负数时
// 回落到 DefaultContextWindow，保证"没配置"和"配错了"都不会崩溃。
// maxTokens 是单次输出上限，它会被从消息预算里扣掉。
func NewBudget(contextWindow, maxTokens int) Budget {
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindow
	}
	return Budget{
		ContextWindow:  contextWindow,
		ThresholdRatio: DefaultThresholdRatio,
		HeadroomTokens: DefaultHeadroomTokens,
		RetainRatio:    DefaultRetainRatio,
		MaxTokens:      maxTokens,
	}
}

// messageBudget 返回可用于消息历史的 token 预算。
func (b Budget) messageBudget() int {
	budget := b.ContextWindow - b.MaxTokens - b.HeadroomTokens - b.ToolsTokens
	if budget <= 0 {
		// 配置极端时退回一个保守值，避免除零与永远触发
		return DefaultContextWindow / 2
	}
	return budget
}

// Threshold 返回触发压缩的 token 水位。
// 同时受"窗口比例"和"消息预算"两者约束，取小。
func (b Budget) Threshold() int {
	byWindow := int(float64(b.ContextWindow) * b.ThresholdRatio)
	byBudget := b.messageBudget()
	if byBudget < byWindow {
		return byBudget
	}
	return byWindow
}

// RetainTokens 返回压缩后需要保留的原始尾段 token 数。
func (b Budget) RetainTokens() int {
	n := int(float64(b.messageBudget()) * b.RetainRatio)
	if n < MinRetainTokens {
		return MinRetainTokens
	}
	return n
}

// NeedCompaction 判断当前消息是否已经超过压缩水位。
func (b Budget) NeedCompaction(messages []provider.LLMMessage) bool {
	return EstimateMessages(messages)+b.ToolsTokens >= b.Threshold()
}

// SelectTailStart 从尾部向前累计 token，找到应保留的原始消息起始下标。
//
// 返回值语义（调用方据此判断是否需要压缩）：
//   - start <= firstCompressible：**无需压缩**（保留预算装得下全部可压缩内容）
//   - start >= len(messages)：无需压缩（同上，保守形态）
//   - 1 < start < len(messages)：从 start 起保留，[firstCompressible, start) 进入压缩
//
// 规则（与 DSH 一致的三条不变量）：
//  1. 绝不压缩 system 提示词
//  2. 绝不切断 assistant(tool_calls) 与它后续 tool 结果的原子对
//  3. 保留预算装得下全部内容时，明确表示"不需要压缩"
func SelectTailStart(messages []provider.LLMMessage, retainTokens int) int {
	if len(messages) == 0 {
		return 0
	}
	firstCompressible := 0
	if messages[0].Role == "system" {
		firstCompressible = 1
	}
	// 除 system 外没有可压缩内容
	if len(messages) <= firstCompressible {
		return firstCompressible
	}

	accumulated := 0
	keepFrom := len(messages)
	for i := len(messages) - 1; i >= firstCompressible; i-- {
		accumulated += EstimateMessage(messages[i])
		keepFrom = i
		if accumulated >= retainTokens {
			break
		}
	}
	// 预算装得下所有消息（没触发 break 就一直累到了 firstCompressible）
	if accumulated < retainTokens && keepFrom <= firstCompressible {
		return firstCompressible
	}
	if keepFrom <= firstCompressible {
		return firstCompressible
	}

	// 向前回退，避开切在工具对中间
	for keepFrom > firstCompressible {
		if isSafeCut(messages, keepFrom) {
			break
		}
		keepFrom--
	}
	// 回退到头部说明整段都可压缩，但没有尾段可留 —— 视为无需压缩，
	// 否则会把"全压缩成一条 checkpoint"这种极端形态强加给调用方
	if keepFrom <= firstCompressible {
		return firstCompressible
	}
	return keepFrom
}

// isSafeCut 判断在 idx 处切分是否安全：
// idx 位置的消息不能是裸 tool 结果（必须与它前面的 assistant 调用同组），
// 也不能让 idx-1 位置的 assistant(tool_calls) 失去它后面的结果。
func isSafeCut(messages []provider.LLMMessage, idx int) bool {
	if idx <= 0 || idx >= len(messages) {
		return true
	}
	// 切点落在 tool 结果上 = 会把结果和它的调用分开
	if messages[idx].Role == "tool" {
		return false
	}
	// 切点前一条是带 tool_calls 的 assistant，且它还有未配对的调用
	if prev := messages[idx-1]; prev.Role == "assistant" && len(prev.ToolCalls) > 0 {
		// 检查 idx 起是否紧跟这些调用的结果
		paired := 0
		for j := idx; j < len(messages) && messages[j].Role == "tool"; j++ {
			paired++
		}
		if paired < len(prev.ToolCalls) {
			return false
		}
	}
	return true
}

// Describe 返回人类可读的预算摘要，用于日志与前端状态展示。
func (b Budget) Describe() string {
	return fmt.Sprintf("window=%d max_out=%d tools=%d threshold=%d retain=%d",
		b.ContextWindow, b.MaxTokens, b.ToolsTokens, b.Threshold(), b.RetainTokens())
}

// JoinSummaryBlock 用统一的分隔线包裹压缩摘要，便于前端识别与模型定位。
func JoinSummaryBlock(header, body string) string {
	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n")
	sb.WriteString(body)
	return sb.String()
}

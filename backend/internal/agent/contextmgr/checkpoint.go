package contextmgr

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
)

// ─── 结构化 checkpoint 摘要（对应 DSH 的 compaction-basic / summarizer）───
//
// MiniPanel 原来的"微压缩"是纯裁剪式的：把中段消息压成一串
// "✓ tool_name: 前 120 字"，信息密度极低，模型继续干活时经常
// 忘了"用户最初要干什么""哪些方案试过不行"。
//
// DSH 的做法是让模型自己产出一份固定骨架的 checkpoint，
// 每一节都必须存在（空的写 (none)），后续轮次直接在它上面继续叠加。
// 这里沿用其骨架，按服务器运维场景重写各节语义。

const (
	// CheckpointOpenTag / CheckpointCloseTag 包裹 checkpoint 正文，
	// 便于下一轮识别"历史里已经有一份 checkpoint"，从而做增量合并而非重写。
	CheckpointOpenTag  = "<checkpoint>"
	CheckpointCloseTag = "</checkpoint>"
)

// CheckpointInstruction 交给摘要模型的指令。
//
// DSH 的关键设计：把摘要请求作为"对话最后一条 user 消息"发出，
// 而不是单独用一个摘要 system prompt —— 这样 provider 的 KV 前缀缓存
// 依然命中（前缀完全相同，只有尾部多了这段指令），摘要调用的成本
// 从"重传整个上下文"降到"只算新指令"。这里复刻这一点。
const CheckpointInstruction = `你现在是本次运维对话的上下文压缩引擎。请把上面的对话压缩成一份结构化 checkpoint，让另一个模型能无损接手工作。

严格输出下面这个 Markdown 结构，每一节都必须保留、按顺序输出，不要增删标题。用简短的项目符号，不要写散文段落。某一节没有内容就写 (none)，绝不能省略整节。

## 用户目标
- [用户最初提出的目标，以及过程中如何演变；目标措辞影响理解时原样引用]

## 当前环境
- [已确认的服务器/容器/服务状态、路径、配置项、版本号等事实]

## 已执行操作
- [按时间顺序列出实际做过的操作及其结果，例如：执行 systemctl restart nginx → 成功]

## 报错与修复
- [报错原文: 原因分析与解决方式；包括失败的尝试，避免后续重复踩坑]

## 待办事项
- [用户明确要求但尚未完成的工作]

## 正在进行
- [压缩发生的这一刻，手上正在做什么]

## 下一步
- [紧接着要做的唯一动作，或 (none)]

## 关键约束
- [重要决策及其理由、用户的偏好与纠正、安全红线、不能动的服务和文件]

规则：
- 保留精确的路径、命令、报错字符串、服务名、端口、IP、配置键与数值，不要改写或概括它们。
- 用户的纠正与明确指令必须如实保留，这是最重要的一节。
- 不要提及"压缩""摘要""上下文不足"这类元信息，就当自己是在做运维交接记录。
- 只输出 checkpoint 正文，不要调用任何工具。`

// CheckpointPreamble 把 checkpoint 包装成"既定背景"，让模型不要复述它。
const CheckpointPreamble = `以下是本次会话早前被自动压缩的上下文。把它当作已经确立的背景事实，直接在它之上继续工作，不要复述、不要致谢、不要提及压缩这件事。接下来的消息是压缩之后的新内容。`

// Summarizer 通过一次独立 LLM 调用生成 checkpoint。
type Summarizer struct {
	provider provider.Provider
	// maxInputTokens 超过此规模时先做确定性裁剪再送摘要，防止摘要请求本身溢出
	maxInputTokens int
}

// NewSummarizer 创建摘要器。maxTokens 为模型上下文窗口。
func NewSummarizer(p provider.Provider, maxTokens int) *Summarizer {
	if maxTokens <= 0 {
		maxTokens = DefaultContextWindow
	}
	return &Summarizer{provider: p, maxInputTokens: maxTokens / 2}
}

// Summarize 对将被压缩的消息段生成 checkpoint 正文。
//
// 失败时返回确定性摘要兜底 —— 压缩这件事本身不能成为会话中断的原因：
// 哪怕摘要调用挂了，也要让对话继续下去。
func (s *Summarizer) Summarize(ctx context.Context, messages []provider.LLMMessage) (string, error) {
	if s == nil || s.provider == nil {
		return DeterministicSummary(messages), fmt.Errorf("no summarizer provider")
	}

	// 摘要请求本身也要控规模：先做一次确定性裁剪
	input := messages
	if EstimateMessages(input) > s.maxInputTokens {
		input, _ = PruneMessages(input)
		if EstimateMessages(input) > s.maxInputTokens {
			// 裁剪还不够就只保留尾部（尾部是最近的现场）
			start := SelectTailStart(input, s.maxInputTokens/2)
			if start > 1 && start < len(input) {
				trimmed := make([]provider.LLMMessage, 0, len(input)-start+1)
				trimmed = append(trimmed, input[0]) // 保留 system
				trimmed = append(trimmed, input[start:]...)
				input = trimmed
			}
		}
	}

	// 关键：复用对话原有前缀（system + 全部历史），只在最后追加指令，
	// 让 provider 的 KV 前缀缓存命中
	req := make([]provider.LLMMessage, 0, len(input)+1)
	req = append(req, input...)
	req = append(req, provider.LLMMessage{Role: "user", Content: CheckpointInstruction})

	resp, err := s.provider.Chat(ctx, req, nil)
	if err != nil {
		global.LOG.Warnf("[ContextMgr] checkpoint 摘要调用失败，回退确定性摘要: %v", err)
		return DeterministicSummary(messages), err
	}
	body := strings.TrimSpace(resp.Content)
	body = stripWrappingFence(body)
	if body == "" {
		return DeterministicSummary(messages), fmt.Errorf("summarizer returned empty content")
	}
	return body, nil
}

// stripWrappingFence 去掉模型偶尔包在外面的 ```markdown 围栏，
// 否则 checkpoint 会被二次包裹，下一轮解析时出现嵌套。
func stripWrappingFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	firstNL := strings.IndexByte(s, '\n')
	if firstNL < 0 {
		return s
	}
	inner := s[firstNL+1:]
	if idx := strings.LastIndex(inner, "```"); idx >= 0 {
		inner = inner[:idx]
	}
	return strings.TrimSpace(inner)
}

// FormatCheckpoint 把 checkpoint 正文包装成一条可注入的 user 消息内容。
func FormatCheckpoint(body string) string {
	return CheckpointPreamble + "\n\n" + CheckpointOpenTag + "\n" + body + "\n" + CheckpointCloseTag
}

// DeterministicSummary 不依赖模型的兜底摘要。
// 它按角色归类，抽取操作与报错的关键行，保证压缩永远能完成。
func DeterministicSummary(messages []provider.LLMMessage) string {
	names := make(map[string]string, len(messages))
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			if tc.ID != "" {
				names[tc.ID] = tc.Function.Name
			}
		}
	}

	var ops, errs, says []string
	for _, m := range messages {
		switch m.Role {
		case "tool":
			name := names[m.ToolCallID]
			if name == "" {
				name = "tool"
			}
			line := oneLine(m.Content, 160)
			if isFailure(m.Content) {
				errs = append(errs, fmt.Sprintf("- %s: %s", name, line))
			} else {
				ops = append(ops, fmt.Sprintf("- %s → %s", name, line))
			}
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var calls []string
				for _, tc := range m.ToolCalls {
					calls = append(calls, tc.Function.Name+"("+oneLine(tc.Function.Arguments, 80)+")")
				}
				ops = append(ops, "- 调用 "+strings.Join(calls, ", "))
			}
			if t := strings.TrimSpace(m.Content); t != "" {
				says = append(says, "- "+oneLine(t, 160))
			}
		case "user":
			says = append(says, "- 用户: "+oneLine(m.Content, 200))
		}
	}

	var sb strings.Builder
	sb.WriteString("## 用户目标\n")
	sb.WriteString(sectionOrNone(says, 4))
	sb.WriteString("\n## 当前环境\n- (未归纳)\n")
	sb.WriteString("\n## 已执行操作\n")
	sb.WriteString(sectionOrNone(ops, 12))
	sb.WriteString("\n## 报错与修复\n")
	sb.WriteString(sectionOrNone(errs, 6))
	sb.WriteString("\n## 待办事项\n- (none)\n")
	sb.WriteString("\n## 正在进行\n- 承接上述操作继续处理\n")
	sb.WriteString("\n## 下一步\n- 根据用户最新指令继续\n")
	sb.WriteString("\n## 关键约束\n- 以下为确定性提取的记录，未经模型归纳\n")
	return sb.String()
}

func sectionOrNone(lines []string, limit int) string {
	if len(lines) == 0 {
		return "- (none)\n"
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:] // 取最近的，前面的已经不重要
	}
	return strings.Join(lines, "\n") + "\n"
}

func oneLine(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if max > 0 && len(s) > max {
		cut := clampIndex(s, max)
		s = s[:cut] + "…"
	}
	return s
}

func isFailure(content string) bool {
	c := strings.TrimSpace(content)
	return strings.HasPrefix(c, "执行失败") ||
		strings.HasPrefix(c, "Error:") ||
		strings.HasPrefix(c, "FAILED") ||
		strings.HasPrefix(c, "exit status") ||
		strings.Contains(c, "command not found")
}

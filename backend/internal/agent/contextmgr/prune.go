package contextmgr

import (
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// ─── 工具结果确定性裁剪（对应 DSH 的 compaction-tool-result-pruner）───
//
// 先裁剪、后摘要。理由：工具输出在运维对话里占绝大多数 token，
// 而其中大部分是"执行成功但内容冗长"（日志、目录列表、配置全文）。
// 这部分根本不需要模型理解，只需保留可辨认的头部与尾部，
// 一次确定性裁剪往往就能把水位降下来，省掉整个摘要调用。

const (
	// pruneHeadBytes 裁剪后保留的头部字节数
	pruneHeadBytes = 900
	// pruneTailBytes 裁剪后保留的尾部字节数
	pruneTailBytes = 500
	// pruneTriggerBytes 超过此长度的工具结果才值得裁剪
	pruneTriggerBytes = 2400
	// pruneMinAge 最近 N 条消息内的工具结果不动（模型正在用）
	pruneMinAge = 6
)

// PruneMessages 裁剪冗长的工具结果，返回新切片与裁剪条数。
//
// 它保持消息条数不变（原地替换 Content），因此绝不会破坏
// tool_call / tool_result 的配对关系 —— 这是能放在摘要之前安全执行的原因。
func PruneMessages(messages []provider.LLMMessage) ([]provider.LLMMessage, int) {
	if len(messages) < 3 {
		return messages, 0
	}
	out := make([]provider.LLMMessage, len(messages))
	copy(out, messages)
	count := 0
	// 最近 pruneMinAge 条不裁剪：模型当前步正在引用它们
	// 注意 cutoff 是"上界"，i >= cutoff 一律跳过
	cutoff := len(out) - pruneMinAge
	for i := 0; i < len(out); i++ {
		if i >= cutoff {
			break
		}
		m := &out[i]
		if m.Role != "tool" {
			continue
		}
		if len(m.Content) <= pruneTriggerBytes {
			continue
		}
		m.Content = prunedToolResult(m.Content)
		count++
	}
	return out, count
}

// prunedToolResult 生成"头 + 省略标记 + 尾"的压缩文本。
//
// 尾部必须保留：运维命令的结论（成功/失败、错误信息）几乎总在最后几行。
func prunedToolResult(content string) string {
	head := TruncateText(content, pruneHeadBytes)
	tail := tailBytes(content, pruneTailBytes)
	omitted := len(content) - len(head) - len(tail)
	if omitted < 0 {
		omitted = 0
	}
	var sb strings.Builder
	sb.WriteString(head)
	sb.WriteString(fmt.Sprintf("\n\n…[此处省略 %d 字节，共 %d 字节]…\n\n", omitted, len(content)))
	sb.WriteString(tail)
	return sb.String()
}

// tailBytes 取文本末尾 n 字节，对齐 UTF-8 边界。
func tailBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && !isRuneStart(s[cut]) {
		cut++
	}
	return s[cut:]
}

func isRuneStart(b byte) bool {
	return b&0xC0 != 0x80
}

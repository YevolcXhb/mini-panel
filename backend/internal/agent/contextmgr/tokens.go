package contextmgr

import (
	"strings"
	"unicode/utf8"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// ─── token 估算 ───
//
// 不引入 tiktoken（CGO 与几十 MB 词典都不适合嵌入式 arm64 部署），
// 用分层启发式估算。它只服务于"要不要压缩"这个判断，
// 精度要求是"不低估中文、不忽略长工具输出"，不需要逐 token 对齐。

// MinToolResultKeep 最近 N 条工具结果永远不裁剪（模型正在用的现场证据）
const MinToolResultKeep = 4

// EstimateMessages 估算一组消息的 token 总量。
func EstimateMessages(messages []provider.LLMMessage) int {
	total := 0
	for _, m := range messages {
		total += EstimateMessage(m)
	}
	return total
}

// EstimateMessage 估算单条消息（含 role 开销与工具调用序列化开销）。
func EstimateMessage(m provider.LLMMessage) int {
	// 每条消息约 4 token 的角色与分隔开销
	n := 4
	n += EstimateText(m.Content)
	for _, tc := range m.ToolCalls {
		n += 8 // 结构化开销
		n += EstimateText(tc.ID)
		n += EstimateText(tc.Function.Name)
		n += EstimateText(tc.Function.Arguments)
	}
	return n
}

// EstimateText 估算纯文本 token。
//
// 规则（经验值，对 DeepSeek/GPT 系 tokenizer 偏差可接受）：
//   - ASCII 可打印字符：约 3.6 字符/token
//   - CJK 字符：约 1.1 字符/token（汉字基本 1 字 1~1.5 token）
//   - 空白：折半计
//   - 其它（emoji/符号）：按 2 token 计
func EstimateText(s string) int {
	if s == "" {
		return 0
	}
	ascii, cjk, other, space := 0, 0, 0, 0
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t':
			space++
		case r == '\n' || r == '\r':
			space += 2
		case r < 0x80:
			if r >= 0x20 && r < 0x7f {
				ascii++
			} else {
				other++
			}
		case r >= 0x3000 && r <= 0x9fff:
			cjk++
		case r >= 0xff00 && r <= 0xffef:
			cjk++
		case r >= 0x20000 && r <= 0x2ffff:
			cjk++
		default:
			other++
		}
	}
	n := 0
	n += (ascii*10 + 35) / 36 // ≈ ascii/3.6
	n += (cjk*10 + 10) / 11   // ≈ cjk/1.1
	n += space / 2
	n += other * 2
	if n == 0 {
		n = 1
	}
	return n
}

// clampIndex 把字节下标对齐到 UTF-8 字符边界，避免截断出乱码。
func clampIndex(s string, max int) int {
	if max <= 0 {
		return 0
	}
	if len(s) <= max {
		return len(s)
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return cut
}

// TruncateText 安全截断文本到 maxBytes 字节（不产生残字）。
func TruncateText(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := clampIndex(s, maxBytes)
	var sb strings.Builder
	sb.WriteString(s[:cut])
	sb.WriteString("\n…[已截断]")
	return sb.String()
}

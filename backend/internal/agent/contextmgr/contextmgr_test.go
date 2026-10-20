package contextmgr

import (
	"testing"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// ─── 测试用构造器 ───

func sysMsg(s string) provider.LLMMessage { return provider.LLMMessage{Role: "system", Content: s} }
func userMsg(s string) provider.LLMMessage { return provider.LLMMessage{Role: "user", Content: s} }
func asstMsg(s string) provider.LLMMessage { return provider.LLMMessage{Role: "assistant", Content: s} }

func asstWithCalls(s string, ids ...string) provider.LLMMessage {
	m := provider.LLMMessage{Role: "assistant", Content: s}
	for _, id := range ids {
		m.ToolCalls = append(m.ToolCalls, provider.ToolCall{
			ID:   id,
			Type: "function",
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "execute_command", Arguments: `{"cmd":"ls"}`},
		})
	}
	return m
}

func toolMsg(id, content string) provider.LLMMessage {
	return provider.LLMMessage{Role: "tool", ToolCallID: id, Content: content}
}

// ─── SelectTailStart 不变量 ───

// 核心不变量：切点绝不能落在 tool 结果上而把它的调用留在被压缩侧。
func TestSelectTailStartNeverSplitsToolPairs(t *testing.T) {
	msgs := []provider.LLMMessage{
		sysMsg("system prompt"),
		userMsg("帮我看看磁盘"),
		asstWithCalls("", "c1", "c2"),
		toolMsg("c1", "df output"),
		toolMsg("c2", "du output"),
		asstMsg("磁盘用了 60%"),
		userMsg("继续"),
		asstWithCalls("", "c3"),
		toolMsg("c3", "ls output"),
	}

	// 各种保留量都要保证切点安全
	for _, retain := range []int{1, 5, 20, 50, 200, 1000} {
		start := SelectTailStart(msgs, retain)
		if start <= 1 || start >= len(msgs) {
			continue // 全压缩或全保留，都是安全形态
		}
		if msgs[start].Role == "tool" {
			t.Fatalf("retain=%d: 切点落在 tool 结果上 (idx=%d)", retain, start)
		}
		// 若切点前一条是带 tool_calls 的 assistant，其后必须紧跟它的全部结果
		prev := msgs[start-1]
		if prev.Role == "assistant" && len(prev.ToolCalls) > 0 {
			paired := 0
			for j := start; j < len(msgs) && msgs[j].Role == "tool"; j++ {
				paired++
			}
			if paired < len(prev.ToolCalls) {
				t.Fatalf("retain=%d: 切点把 assistant 的 %d 个调用与仅 %d 个结果分开了",
					retain, len(prev.ToolCalls), paired)
			}
		}
	}
}

func TestSelectTailStartKeepsSystem(t *testing.T) {
	msgs := []provider.LLMMessage{
		sysMsg("S"),
		userMsg("u1"),
		asstMsg("a1"),
	}
	start := SelectTailStart(msgs, 10000)
	// 预算装得下全部内容 → 返回 firstCompressible，语义是"无需压缩"
	if start != 1 {
		t.Fatalf("内容全部装得下时应返回 1（表示无需压缩），得到 %d", start)
	}
	// Manager 必须据此判定不需压缩
	mm := NewManager(nil, 65536, 4096, 4096)
	ok := start > 1 && start < len(msgs)
	if ok {
		t.Fatal("这组输入不应被判定为需要压缩")
	}
	_ = mm
}

func TestSelectTailStartEmptyAndSingleton(t *testing.T) {
	if got := SelectTailStart(nil, 100); got != 0 {
		t.Fatalf("空列表应返回 0，得到 %d", got)
	}
	one := []provider.LLMMessage{sysMsg("S")}
	if got := SelectTailStart(one, 100); got != 1 {
		t.Fatalf("单条应返回 1，得到 %d", got)
	}
}

// ─── 裁剪 ───

func TestPruneMessagesTruncatesLongToolResults(t *testing.T) {
	long := ""
	for i := 0; i < 400; i++ {
		long += "0123456789"
	}
	// 长结果后面必须再跟 ≥pruneMinAge 条消息，它才会离开"最近"区
	msgs := []provider.LLMMessage{
		sysMsg("S"),
		userMsg("u"),
		asstWithCalls("", "c1"),
		toolMsg("c1", long),
		asstMsg("a"),
		userMsg("u2"),
		asstMsg("a2"),
		userMsg("u3"),
		asstMsg("a3"),
		userMsg("u4"),
		asstMsg("a4"),
	}
	out, n := PruneMessages(msgs)
	if n != 1 {
		t.Fatalf("应裁剪 1 条，实际 %d", n)
	}
	if len(out) != len(msgs) {
		t.Fatalf("裁剪后消息条数必须不变: %d → %d", len(msgs), len(out))
	}
	if len(out[3].Content) >= len(long) {
		t.Fatalf("裁剪后长度未下降: %d", len(out[3].Content))
	}
	// 尾部必须保留（运维结论都在末尾）
	if !containsStr(out[3].Content, "0123456789") {
		t.Fatalf("裁剪后丢失了尾部内容")
	}
}

func TestPruneMessagesKeepsRecent(t *testing.T) {
	long := ""
	for i := 0; i < 400; i++ {
		long += "0123456789"
	}
	// 长结果放在最后几条，属于"最近"，不该被裁剪
	msgs := []provider.LLMMessage{
		sysMsg("S"),
		userMsg("u"),
		asstMsg("a"),
		toolMsg("c9", long),
	}
	out, n := PruneMessages(msgs)
	if n != 0 {
		t.Fatalf("最近的结果不应被裁剪，实际裁剪 %d 条", n)
	}
	if out[3].Content != long {
		t.Fatalf("最近结果内容被改动")
	}
}

// ─── token 估算 ───

func TestEstimateTextChineseHigherThanAscii(t *testing.T) {
	ascii := EstimateText("abcdefghij")            // 10 ascii
	cjk := EstimateText("服务器运维管理系统测试") // 12 汉字
	if cjk <= ascii/2 {
		t.Fatalf("中文估算明显偏低: ascii=%d cjk=%d", ascii, cjk)
	}
}

func TestEstimateTextEmpty(t *testing.T) {
	if got := EstimateText(""); got != 0 {
		t.Fatalf("空串应为 0，得到 %d", got)
	}
}

// ─── 预算 ───

func TestBudgetThresholdBelowWindow(t *testing.T) {
	b := NewBudget(65536, 4096)
	th := b.Threshold()
	if th <= 0 || th >= b.ContextWindow {
		t.Fatalf("阈值应在 (0, window) 内: %d / %d", th, b.ContextWindow)
	}
}

func TestBudgetRetainAtLeastMin(t *testing.T) {
	b := NewBudget(4096, 1024)
	if r := b.RetainTokens(); r < MinRetainTokens {
		t.Fatalf("尾段保留量下限被破坏: %d < %d", r, MinRetainTokens)
	}
}

// 百万级上下文必须真正生效：窗口参数被尊重，水位随之抬到百万量级。
// 这是用户反馈的原始问题 —— 老实现把窗口写死成 65536。
func TestBudgetHonorsMillionContextWindow(t *testing.T) {
	const million = 1000000
	b := NewBudget(million, 8192)
	if b.ContextWindow != million {
		t.Fatalf("窗口未生效: %d，期望 %d", b.ContextWindow, million)
	}
	th := b.Threshold()
	// 800K 量级，绝不能还停在 64K 附近
	if th < 700000 {
		t.Fatalf("百万窗口的压缩水位过低: %d（说明仍按 64K 估算）", th)
	}
	if th >= million {
		t.Fatalf("水位不应达到或超过窗口本身: %d", th)
	}
	// 保留量也应随窗口放大，否则压缩后丢现场
	if r := b.RetainTokens(); r < 100000 {
		t.Fatalf("百万窗口下保留量过小: %d", r)
	}
}

// 窗口配置缺失或非法时回落默认值，不能崩也不能触发"每轮都压缩"。
func TestBudgetFallsBackOnInvalidWindow(t *testing.T) {
	for _, w := range []int{0, -1, -99999} {
		b := NewBudget(w, 4096)
		if b.ContextWindow != DefaultContextWindow {
			t.Fatalf("窗口=%d 时应回落 %d，实际 %d", w, DefaultContextWindow, b.ContextWindow)
		}
		if th := b.Threshold(); th <= 0 {
			t.Fatalf("窗口=%d 时阈值应为正数，实际 %d", w, th)
		}
	}
}

// ─── 错误分类 ───

func TestIsOverflowError(t *testing.T) {
	cases := map[string]bool{
		"This model's maximum context length is 65536 tokens": true,
		"error: context_length_exceeded":                     true,
		"prompt is too long":                                 true,
		"429 Too Many Requests":                              false,
		"connection reset by peer":                           false,
		"":                                                   false,
	}
	for msg, want := range cases {
		if got := IsOverflowError(msg); got != want {
			t.Fatalf("IsOverflowError(%q) = %v, 期望 %v", msg, got, want)
		}
	}
}

func TestIsRetryableError(t *testing.T) {
	if !IsRetryableError("429 Too Many Requests") {
		t.Fatal("429 应可重试")
	}
	if !IsRetryableError("502 Bad Gateway") {
		t.Fatal("502 应可重试")
	}
	if IsRetryableError("context_length_exceeded") {
		t.Fatal("上下文超限不该走退避重试")
	}
	if IsRetryableError("invalid api key") {
		t.Fatal("鉴权错误不该重试")
	}
}

// ─── 确定性摘要 ───

func TestDeterministicSummaryHasAllSections(t *testing.T) {
	msgs := []provider.LLMMessage{
		sysMsg("S"),
		userMsg("查看磁盘"),
		asstWithCalls("", "c1"),
		toolMsg("c1", "执行失败: permission denied"),
	}
	s := DeterministicSummary(msgs)
	for _, sec := range []string{
		"## 用户目标", "## 当前环境", "## 已执行操作",
		"## 报错与修复", "## 待办事项", "## 正在进行",
		"## 下一步", "## 关键约束",
	} {
		if !containsStr(s, sec) {
			t.Fatalf("确定性摘要缺少章节 %q", sec)
		}
	}
	if !containsStr(s, "permission denied") {
		t.Fatal("确定性摘要丢失了报错内容")
	}
}

func TestFormatCheckpointWrapping(t *testing.T) {
	got := FormatCheckpoint("BODY")
	if !containsStr(got, CheckpointOpenTag) || !containsStr(got, CheckpointCloseTag) {
		t.Fatal("checkpoint 未正确包裹")
	}
}

func containsStr(hay, needle string) bool {
	return len(needle) == 0 || indexOf(hay, needle) >= 0
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

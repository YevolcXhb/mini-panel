package agentcore

import (
	"sync"
	"time"
)

// ─── 运行中插话（inbox / step-boundary input）───
//
// DSH 的做法值得照搬：用户在 agent 跑动过程中发出的消息不进 SSE 的请求通道，
// 而是先进 inbox 队列，由主循环在**步边界**（一次工具批次全部执行完、
// 下一次调 LLM 之前）一次性取走并注入上下文。
//
// 为什么不直接在 token 流里插入：
//  1. 工具调用批次是有原子性的 —— 一批 3 个 tool_call 必须攒齐 3 个结果，
//     中途插入 user 消息会让 assistant(tool_calls) 与 tool 结果不再相邻，
//     API 会直接 400。
//  2. LLM 正在流式生成时不接受新输入，插进去只能等下一次请求，
//     所以"边界注入"不是妥协，而是唯一正确的语义。
//
// 为什么不用 channel：需要"查看有没有积压但不消费"（前端展示排队状态）、
// 需要按会话隔离、需要能被多次 Run 复用（一次运行结束后残留的插话要清掉）。
// 用带锁的切片最简单也最可控。

// InboxMessage 一条待注入的插话。
type InboxMessage struct {
	// Content 文本内容
	Content string
	// QueuedAt 入队时间（前端显示排队时长）
	QueuedAt time.Time
}

// Inbox 单会话的插话队列。
type Inbox struct {
	mu   sync.Mutex
	msgs []InboxMessage
	// total 累计入队数（用于前端统计）
	total int
}

// NewInbox 创建队列。
func NewInbox() *Inbox {
	return &Inbox{}
}

// Push 入队一条插话。内容 trim 后为空则忽略并返回 false。
func (i *Inbox) Push(content string) bool {
	c := trimSpace(content)
	if c == "" {
		return false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.msgs == nil {
		i.msgs = make([]InboxMessage, 0, 4)
	}
	i.msgs = append(i.msgs, InboxMessage{Content: c, QueuedAt: time.Now()})
	i.total++
	return true
}

// Drain 取走全部积压插话，返回文本切片。无积压时返回 nil。
func (i *Inbox) Drain() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.msgs) == 0 {
		return nil
	}
	out := make([]string, len(i.msgs))
	for idx, m := range i.msgs {
		out[idx] = m.Content
	}
	i.msgs = i.msgs[:0]
	return out
}

// Pending 返回当前积压条数（不消费），用于前端展示"已排队 N 条"。
func (i *Inbox) Pending() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.msgs)
}

// Total 返回本次运行累计收到的插话数。
func (i *Inbox) Total() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.total
}

// Clear 清空队列与计数（一次运行结束时调用）。
func (i *Inbox) Clear() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.msgs = nil
	i.total = 0
}

// Snapshot 返回积压内容副本，供调试接口使用。
func (i *Inbox) Snapshot() []InboxMessage {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]InboxMessage, len(i.msgs))
	copy(out, i.msgs)
	return out
}

// ─── 会话 → inbox 注册表 ───

// InboxRegistry 按会话 ID 管理 inbox。
//
// 生命期与 Engine 无关：API 层收到插话时 Engine 可能正在跑，
// 所以队列必须挂在比 Engine 更长寿的注册表上。
type InboxRegistry struct {
	mu     sync.Mutex
	queues map[uint]*Inbox
}

var globalInboxes = &InboxRegistry{queues: make(map[uint]*Inbox)}

// DefaultInboxes 返回全局注册表。
func DefaultInboxes() *InboxRegistry { return globalInboxes }

// Get 取（必要时创建）某会话的 inbox。
func (r *InboxRegistry) Get(sessionID uint) *Inbox {
	r.mu.Lock()
	defer r.mu.Unlock()
	if q, ok := r.queues[sessionID]; ok {
		return q
	}
	q := NewInbox()
	r.queues[sessionID] = q
	return q
}

// Lookup 只查不建，未注册时返回 nil。
func (r *InboxRegistry) Lookup(sessionID uint) *Inbox {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.queues[sessionID]
}

// Drop 会话删除时移除队列，避免长期泄漏。
func (r *InboxRegistry) Drop(sessionID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.queues, sessionID)
}

// trimSpace 轻量去空白（避免为一个函数引入 strings 之外的依赖链）。
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && isSpaceByte(s[start]) {
		start++
	}
	for end > start && isSpaceByte(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

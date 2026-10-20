package api

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/minipanel/minipanel/internal/agent"
	"github.com/minipanel/minipanel/internal/agent/agentcore"
)

// 回归：向已关闭的通道发送不能 panic。
//
// 历史事故：Chat 的内层转发 goroutine 未被等待，外层 defer 先 close(stream)，
// 内层随后执行 stream <- chunk，触发 panic: send on closed channel。
// panic 发生在普通 goroutine 中会终止整个进程 —— 面板因此周期性崩溃重启。
func TestSendChunkOnClosedChannelDoesNotPanic(t *testing.T) {
	stream := make(chan agent.StreamChunk, 1)
	close(stream)

	sent := sendChunk(context.Background(), stream, agent.StreamChunk{Type: "text"})
	if sent {
		t.Fatal("通道已关闭，返回值应为 false")
	}
}

// ctx 取消后应立即放弃发送，而不是一直阻塞
func TestSendChunkRespectsCancelledContext(t *testing.T) {
	stream := make(chan agent.StreamChunk) // 无缓冲且无人读取
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan bool, 1)
	go func() {
		done <- sendChunk(ctx, stream, agent.StreamChunk{Type: "text"})
	}()

	select {
	case sent := <-done:
		if sent {
			t.Fatal("ctx 已取消时不应发送成功")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 sendChunk 应立刻返回，实际阻塞")
	}
}

// 关键回归：dst 已被提前关闭时，forwardChunks 必须不 panic 且能排空 src
func TestForwardChunksAfterDstClosed(t *testing.T) {
	ctx := context.Background()
	dst := make(chan agent.StreamChunk, 1)
	close(dst) // 模拟外层提前关流

	src := make(chan agentcore.StreamChunk, 4)
	src <- agentcore.StreamChunk{Type: "text", Content: "a"}
	src <- agentcore.StreamChunk{Type: "text", Content: "b"}
	close(src)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dst 关闭后转发仍 panic: %v", r)
		}
	}()
	forwardChunks(ctx, src, dst)
}

// runAndForward 返回时转发必须已退出，因此随后 close(dst) 安全，且消息不丢
func TestRunAndForwardDeliversAllThenSafeToClose(t *testing.T) {
	ctx := context.Background()
	dst := make(chan agent.StreamChunk, 8)

	var got int
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range dst {
			got++
		}
	}()

	const total = 50
	runAndForward(ctx, func(ac chan agentcore.StreamChunk) error {
		for i := 0; i < total; i++ {
			ac <- agentcore.StreamChunk{Type: "text", Content: "x"}
		}
		return nil
	}, dst)

	// 关键断言：走到这里说明转发已退出，关流不会与发送竞争
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("runAndForward 返回后 close(dst) 仍 panic: %v", r)
		}
	}()
	close(dst)
	wg.Wait()

	if got != total {
		t.Fatalf("消息丢失：期望 %d，实际 %d", total, got)
	}
}

// ctx 取消后必须能排空 src，避免上游生产者永久阻塞
func TestRunAndForwardDrainsAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 一开始就取消

	dst := make(chan agent.StreamChunk, 4)
	const total = 40

	done := make(chan struct{})
	go func() {
		defer close(done)
		runAndForward(ctx, func(ac chan agentcore.StreamChunk) error {
			for i := 0; i < total; i++ {
				ac <- agentcore.StreamChunk{Type: "text"}
			}
			return nil
		}, dst)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ctx 取消后 runAndForward 未能返回，可能发生死锁")
	}
}

// toAgentChunk 字段映射与 usage 转换必须完整
func TestToAgentChunkMapsFields(t *testing.T) {
	in := agentcore.StreamChunk{
		Type:       "usage",
		Content:    "c",
		ToolCallID: "tid",
		ToolName:   "tn",
		Command:    "cmd",
		Message:    "m",
		Error:      "e",
		Success:    true,
		Cached:     true,
		Phase:      "p",
		StepNumber: 3,
		MaxSteps:   24,
		Usage: &agentcore.StreamUsage{
			Tokens: 100, Threshold: 80, ContextWindow: 1000, Percent: 10, Compactions: 2,
		},
	}
	out := toAgentChunk(in)

	if out.Type != in.Type || out.Content != in.Content || out.ToolCallID != in.ToolCallID ||
		out.ToolName != in.ToolName || out.Command != in.Command || out.Message != in.Message ||
		out.Error != in.Error || out.Success != in.Success || out.Cached != in.Cached ||
		out.Phase != in.Phase || out.StepNumber != in.StepNumber || out.MaxSteps != in.MaxSteps {
		t.Fatalf("基础字段映射不完整: %+v", out)
	}
	if out.Usage == nil {
		t.Fatal("Usage 不应为 nil")
	}
	if out.Usage.Tokens != 100 || out.Usage.Threshold != 80 || out.Usage.ContextWindow != 1000 ||
		out.Usage.Percent != 10 || out.Usage.Compactions != 2 {
		t.Fatalf("Usage 字段映射错误: %+v", out.Usage)
	}
}

// Usage 为 nil 时不能解引用导致 panic
func TestToAgentChunkNilUsage(t *testing.T) {
	out := toAgentChunk(agentcore.StreamChunk{Type: "text"})
	if out.Usage != nil {
		t.Fatal("nil Usage 应保持 nil")
	}
}

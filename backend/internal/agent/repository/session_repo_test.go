package repository

import (
	"testing"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/sirupsen/logrus"
)

func init() {
	global.LOG = logrus.New()
}

func toolMsg(id, content string) provider.LLMMessage {
	return provider.LLMMessage{Role: "tool", ToolCallID: id, Content: content}
}

func assistantWithCalls(content string, calls ...provider.ToolCall) provider.LLMMessage {
	return provider.LLMMessage{Role: "assistant", Content: content, ToolCalls: calls}
}

func TestNormalizeMessageSequenceFixesOrphanToolCall(t *testing.T) {
	sm := &SessionManager{}
	// 模拟强制停止后的坏状态：assistant(tool_calls) 后面没有紧跟响应，
	// 占位/真实 tool 消息被追加到了会话末尾
	messages := []provider.LLMMessage{
		{Role: "user", Content: "帮我看看磁盘"},
		assistantWithCalls("", provider.ToolCall{ID: "call_a", Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "execute_command", Arguments: "{}"}}),
		{Role: "user", Content: "后来又问了别的问题"},
		toolMsg("call_a", "执行结果"),
	}

	normalized := sm.normalizeMessageSequence(1, messages)

	// assistant(tool_calls) 后必须立即跟上对应 tool 响应
	for i := 0; i < len(normalized); i++ {
		if normalized[i].Role == "assistant" && len(normalized[i].ToolCalls) > 0 {
			if i+1 >= len(normalized) || normalized[i+1].Role != "tool" || normalized[i+1].ToolCallID != normalized[i].ToolCalls[0].ID {
				t.Fatalf("assistant tool_calls 后缺少紧跟的 tool 响应: %+v", normalized)
			}
		}
	}
}

func TestNormalizeMessageSequenceDedupAndPlaceholder(t *testing.T) {
	sm := &SessionManager{}
	call := provider.ToolCall{ID: "call_a", Function: struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "execute_command", Arguments: "{}"}}

	// 同一 ID 出现占位 + 真实结果时取最后一条（真实结果），并补缺失的 call_b 占位
	messages := []provider.LLMMessage{
		assistantWithCalls("", call, provider.ToolCall{ID: "call_b", Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "read_file", Arguments: "{}"}}),
		toolMsg("call_a", "[操作已中断：用户停止了回复]"),
		toolMsg("call_a", "真实结果"),
	}
	normalized := sm.normalizeMessageSequence(1, messages)

	if len(normalized) != 3 {
		t.Fatalf("expected 3 messages, got %d: %+v", len(normalized), normalized)
	}
	// 顺序：assistant, tool(call_a 真实), tool(call_b 占位)
	if normalized[1].ToolCallID != "call_a" || normalized[1].Content != "真实结果" {
		t.Fatalf("call_a 应取最后一条真实结果: %+v", normalized[1])
	}
	if normalized[2].ToolCallID != "call_b" {
		t.Fatalf("call_b 应补占位: %+v", normalized)
	}
}

func TestNormalizeMessageSequenceDropsOrphanTool(t *testing.T) {
	sm := &SessionManager{}
	messages := []provider.LLMMessage{
		{Role: "user", Content: "hi"},
		toolMsg("orphan", "没有所属 assistant 的 tool 消息"),
	}
	normalized := sm.normalizeMessageSequence(1, messages)
	if len(normalized) != 1 || normalized[0].Role != "user" {
		t.Fatalf("孤儿 tool 消息应被丢弃: %+v", normalized)
	}
}

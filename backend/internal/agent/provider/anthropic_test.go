package provider

import "testing"

func TestClaudeMessagesFromLLM(t *testing.T) {
	tc := ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "get_system_info", Arguments: `{}`},
	}
	messages := []LLMMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "思考中", ToolCalls: []ToolCall{tc}},
		{Role: "tool", ToolCallID: "call_1", Content: "结果"},
	}

	systemPrompt, claude := claudeMessagesFromLLM(messages)
	if systemPrompt != "sys" {
		t.Fatalf("system prompt mismatch: %q", systemPrompt)
	}
	if len(claude) != 3 {
		t.Fatalf("expected 3 claude messages, got %d: %+v", len(claude), claude)
	}

	// assistant 消息必须携带 tool_use 内容块
	blocks, ok := claude[1]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("assistant content should be block array: %+v", claude[1])
	}
	foundToolUse := false
	for _, b := range blocks {
		if b["type"] == "tool_use" && b["id"] == "call_1" && b["name"] == "get_system_info" {
			foundToolUse = true
		}
	}
	if !foundToolUse {
		t.Fatalf("assistant blocks missing tool_use: %+v", blocks)
	}

	// tool 结果必须是带 tool_use_id 的 user 消息
	content, ok := claude[2]["content"].([]map[string]interface{})
	if !ok || len(content) != 1 {
		t.Fatalf("tool result should be a single block: %+v", claude[2])
	}
	if content[0]["type"] != "tool_result" || content[0]["tool_use_id"] != "call_1" {
		t.Fatalf("tool_result block mismatch: %+v", content[0])
	}
}

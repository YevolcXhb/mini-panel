package agent

import "fmt"

// StreamChunk SSE 流式响应分片
type StreamChunk struct {
	Type       string `json:"type"`        // message/tool_call/tool_result/confirm_required/error/done
	Content    string `json:"content"`     // 内容/结果
	ToolCallID string `json:"tool_call_id"` // 工具调用 ID
	ToolName   string `json:"tool_name"`    // 工具名称
	Command    string `json:"command"`      // 需要确认的命令
	Message    string `json:"message"`      // 确认提示消息
	Error      string `json:"error"`        // 错误信息
	Success    bool   `json:"success"`      // 是否成功
}

// ConfirmRequiredError 需要用户确认的错误
type ConfirmRequiredError struct {
	Command string
	Message string
}

func (e *ConfirmRequiredError) Error() string {
	return fmt.Sprintf("需要确认: %s", e.Message)
}

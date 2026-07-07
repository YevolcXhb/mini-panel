package agent

import "fmt"

// StreamChunk SSE 流式响应分片
type StreamChunk struct {
	Type       string `json:"type"`        // message/tool_call/tool_result/confirm_required/error/done/phase_start/phase_complete/plan_ready/compression_triggered
	Content    string `json:"content"`     // 内容/结果
	ToolCallID string `json:"tool_call_id"` // 工具调用 ID
	ToolName   string `json:"tool_name"`    // 工具名称
	Command    string `json:"command"`      // 需要确认的命令
	Message    string `json:"message"`      // 确认提示消息
	Error      string `json:"error"`        // 错误信息
	Success    bool   `json:"success"`      // 是否成功
	// 三阶段编排扩展字段
	Phase       string `json:"phase,omitempty"`        // 当前阶段名（planning/coding/reviewing）
	Plan        string `json:"plan,omitempty"`         // 计划内容（plan_ready 事件）
	TokensSaved int    `json:"tokens_saved,omitempty"` // 压缩节省的 token 数
	StepNumber  int    `json:"step_number,omitempty"`  // 当前步数
	MaxSteps    int    `json:"max_steps,omitempty"`    // 最大步数
}

// ConfirmRequiredError 需要用户确认的错误
type ConfirmRequiredError struct {
	Command string
	Message string
}

func (e *ConfirmRequiredError) Error() string {
	return fmt.Sprintf("需要确认: %s", e.Message)
}

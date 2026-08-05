package agent

import "fmt"

// StreamChunk SSE 流式响应分片
type StreamChunk struct {
	// type 取值：
	//   token           - LLM 流式生成的文本增量（真正的逐 token 推送，前端累积到打字机缓冲）
	//   message         - 完整文本消息（非流式回退路径，或最终总结）
	//   tool_call       - 工具调用开始
	//   tool_result     - 工具执行结果
	//   confirm_required - 需要用户确认
	//   error / done    - 错误 / 流结束
	//   phase_start / phase_complete - 三阶段编排
	//   plan_ready      - 计划已生成，等待用户确认
	//   compression_triggered - 上下文压缩已触发
	Type       string `json:"type"`
	Content    string `json:"content"`          // 内容/结果（token 增量也用此字段）
	ToolCallID string `json:"tool_call_id"`     // 工具调用 ID
	ToolName   string `json:"tool_name"`        // 工具名称
	Command    string `json:"command"`          // 需要确认的命令
	Message    string `json:"message"`          // 确认提示消息
	Error      string `json:"error"`            // 错误信息
	Success    bool   `json:"success"`          // 是否成功
	Cached     bool   `json:"cached,omitempty"` // 工具结果是否来自缓存
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

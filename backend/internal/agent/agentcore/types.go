package agentcore

// StreamChunk SSE 流式响应分片。
// 与 agent.StreamChunk 结构完全一致（字段名与 JSON tag 相同），
// 这样 API 层可以无缝互换，前端协议不用改。
//
// type 取值：
//   token                - LLM 流式文本增量
//   message              - 完整文本消息（非流式回退或最终总结）
//   tool_call            - 工具调用开始
//   tool_result          - 工具执行结果
//   confirm_required     - 需要用户确认
//   compression          - 上下文压缩发生（带 Usage，前端可画占用条）
//   user_input_received  - 运行中插话已在步边界注入上下文
//   error / done         - 错误 / 流结束
type StreamChunk struct {
	Type       string `json:"type"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Command    string `json:"command"`
	Message    string `json:"message"`
	Error      string `json:"error"`
	Success    bool   `json:"success"`
	Cached     bool   `json:"cached,omitempty"`
	Phase      string `json:"phase,omitempty"`

	TokensSaved int          `json:"tokens_saved,omitempty"`
	StepNumber  int          `json:"step_number,omitempty"`
	MaxSteps    int          `json:"max_steps,omitempty"`
	Usage       *StreamUsage `json:"usage,omitempty"`
}

// StreamUsage 上下文占用快照。
type StreamUsage struct {
	Tokens        int     `json:"tokens"`
	Threshold     int     `json:"threshold"`
	ContextWindow int     `json:"context_window"`
	Percent       float64 `json:"percent"`
	Compactions   int     `json:"compactions"`
}

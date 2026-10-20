package model

// AgentConfig 存储 LLM 配置（每个用户一条）
type AgentConfig struct {
	BaseModel
	UserID                  uint    `json:"-" gorm:"uniqueIndex"`
	Provider                string  `json:"provider" gorm:"default:openai"` // openai / anthropic / deepseek / ollama / custom
	BaseURL                 string  `json:"base_url"`
	APIKey                  string  `json:"api_key"` // 写入时接收，读取时不返回（由 API 层控制）
	Model                   string  `json:"model" gorm:"default:gpt-4o-mini"`
	Temperature             float64 `json:"temperature" gorm:"default:0.3"`
	MaxTokens               int     `json:"max_tokens" gorm:"default:4096"`
	Enabled                 bool    `json:"enabled" gorm:"default:true"`
	SystemPrompt            string  `json:"system_prompt" gorm:"type:text"`
	Skills                  string  `json:"skills" gorm:"type:text;default:'[\"system\",\"container\",\"website\",\"database\",\"firewall\",\"file\",\"backup\",\"web\"]'"` // JSON 数组
	AllowDangerousCommands  bool    `json:"allow_dangerous_commands" gorm:"default:false"` // 允许危险命令自动执行（关闭时拦截并询问用户）
	ExecTimeoutSeconds      int     `json:"exec_timeout_seconds" gorm:"default:120"`        // 工具执行超时（秒），默认 120
	ContextWindow           int     `json:"context_window" gorm:"default:65536"`            // 模型上下文窗口（token），用于压缩水位计算
	MaxSteps                int     `json:"max_steps" gorm:"default:24"`                     // 单轮最大 ReAct 步数
	// UseOrchestrator 是否启用三阶段编排（规划→编码→审查）。
	// 纯 agent 模式下建议关闭：运维任务多为直接执行，编排会多消耗 2 轮 LLM 调用。
	UseOrchestrator bool `json:"use_orchestrator" gorm:"default:false"`
}

// AgentSession 会话
type AgentSession struct {
	BaseModel
	UserID   uint           `json:"-" gorm:"index"`
	Title    string         `json:"title" gorm:"default:新会话"`
	Messages []AgentMessage `json:"messages" gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE;"`
}

// AgentMessage 消息
type AgentMessage struct {
	BaseModel
	SessionID  uint   `json:"session_id" gorm:"index"`
	Role       string `json:"role"` // system / user / assistant / tool
	Content    string `json:"content" gorm:"type:text"`
	ToolCalls  string `json:"tool_calls" gorm:"type:text"` // JSON
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	ToolResult string `json:"tool_result" gorm:"type:text"`
}

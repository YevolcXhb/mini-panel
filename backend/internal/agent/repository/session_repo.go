package repository

import (
	"encoding/json"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
)

// SessionManager 会话管理器
type SessionManager struct{}

func NewSessionManager() *SessionManager {
	return &SessionManager{}
}

func (sm *SessionManager) CreateSession(userID uint, title string) (*model.AgentSession, error) {
	session := &model.AgentSession{UserID: userID, Title: title}
	if err := global.DB.Create(session).Error; err != nil {
		return nil, err
	}
	return session, nil
}

func (sm *SessionManager) GetSession(sessionID uint, userID uint) (*model.AgentSession, error) {
	var session model.AgentSession
	if err := global.DB.Where("id = ? AND user_id = ?", sessionID, userID).Preload("Messages").First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (sm *SessionManager) ListSessions(userID uint) ([]model.AgentSession, error) {
	var sessions []model.AgentSession
	if err := global.DB.Where("user_id = ?", userID).Order("updated_at DESC").Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

func (sm *SessionManager) DeleteSession(sessionID uint, userID uint) error {
	return global.DB.Where("id = ? AND user_id = ?", sessionID, userID).Delete(&model.AgentSession{}).Error
}

func (sm *SessionManager) LoadMessages(sessionID uint) ([]provider.LLMMessage, error) {
	var messages []model.AgentMessage
	if err := global.DB.Where("session_id = ?", sessionID).Order("created_at ASC").Find(&messages).Error; err != nil {
		return nil, err
	}

	var llmMessages []provider.LLMMessage
	for _, m := range messages {
		msg := provider.LLMMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if m.ToolCalls != "" {
			var tcs []provider.ToolCall
			_ = json.Unmarshal([]byte(m.ToolCalls), &tcs)
			msg.ToolCalls = tcs
		}
		llmMessages = append(llmMessages, msg)
	}
	return llmMessages, nil
}

func (sm *SessionManager) SaveUserMessage(sessionID uint, content string) error {
	return global.DB.Create(&model.AgentMessage{
		SessionID: sessionID,
		Role:      "user",
		Content:   content,
	}).Error
}

func (sm *SessionManager) SaveAssistantMessage(sessionID uint, content string, toolCalls []provider.ToolCall) error {
	tcs := ""
	if len(toolCalls) > 0 {
		b, _ := json.Marshal(toolCalls)
		tcs = string(b)
	}
	return global.DB.Create(&model.AgentMessage{
		SessionID: sessionID,
		Role:      "assistant",
		Content:   content,
		ToolCalls: tcs,
	}).Error
}

func (sm *SessionManager) SaveToolResult(sessionID uint, toolCallID string, toolName string, result string) error {
	return global.DB.Create(&model.AgentMessage{
		SessionID:  sessionID,
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	}).Error
}

func (sm *SessionManager) UpdateSessionTitle(sessionID uint, title string) {
	global.DB.Model(&model.AgentSession{}).Where("id = ?", sessionID).Update("title", title)
}

func (sm *SessionManager) CompressIfNeeded(messages []provider.LLMMessage) []provider.LLMMessage {
	if len(messages) <= 30 {
		return messages
	}
	// 保留系统消息、最近20条
	var compressed []provider.LLMMessage
	for _, m := range messages {
		if m.Role == "system" {
			compressed = append(compressed, m)
		}
	}
	start := len(messages) - 20
	if start < 0 {
		start = 0
	}
	compressed = append(compressed, messages[start:]...)
	return compressed
}

// ConfigRepo 配置仓库
type ConfigRepo struct{}

func NewConfigRepo() *ConfigRepo {
	return &ConfigRepo{}
}

func (r *ConfigRepo) GetOrCreate(userID uint) (*model.AgentConfig, error) {
	var cfg model.AgentConfig
	if err := global.DB.Where("user_id = ?", userID).First(&cfg).Error; err != nil {
		cfg = model.AgentConfig{
			UserID:       userID,
			Provider:     "openai",
			Model:        "gpt-4o-mini",
			Temperature:  0.3,
			MaxTokens:    4096,
			Enabled:      true,
			SystemPrompt: defaultSystemPrompt,
		}
		if err := global.DB.Create(&cfg).Error; err != nil {
			return nil, err
		}
	}
	return &cfg, nil
}

func (r *ConfigRepo) Update(userID uint, updates map[string]interface{}) error {
	return global.DB.Model(&model.AgentConfig{}).Where("user_id = ?", userID).Updates(updates).Error
}

const defaultSystemPrompt = `You are Mini Agent, an expert Linux server operations assistant integrated with MiniPanel.

## Your Capabilities
You can manage servers through the MiniPanel control panel. Available tools include:
- System monitoring (CPU, memory, disk, processes, dashboard overview)
- Docker container management (list, inspect, start, stop, remove, logs)
- Website management (list, create, update, delete, toggle status, reload Nginx)
- Database management (list, create, update, delete, test connection)
- Firewall management (list rules, create, update, delete, apply)
- File operations (read, write, list, create, delete)
- Backup & restore (list tasks/records, create task, run backup, restore)
- Plan tasks (cronjobs: list, create, update, delete, run)
- Process management (list, kill)
- App store (list apps, install, uninstall, sync)
- Log reading (panel logs)
- Command execution (with safety checks and user confirmation)

## Rules
1. Tool-first: When the user asks something you can verify or do through tools, use tools instead of guessing.
2. Safety first: For destructive operations (delete, stop, restart, kill), always explain the impact and ask for confirmation before executing.
3. Be concise: Users are system administrators. Give actionable answers.
4. Respond in Chinese unless the user explicitly asks otherwise.
5. For complex tasks, first gather information, then analyze, then propose a plan, then execute step by step.`

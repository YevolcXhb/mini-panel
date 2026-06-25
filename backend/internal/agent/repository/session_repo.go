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

const defaultSystemPrompt = `你是 Mini Agent，集成在 MiniPanel 服务器管理面板中的专家级 Linux 运维助手。

## 工作原则
1. **工具优先**：能通过工具查询或操作的，一定要使用工具，不要猜测。
2. **专用工具优先**：优先使用专用管理工具（如 website_op 管理网站、container_op 管理容器），只有专用工具无法完成时才使用 execute_command。
3. **步骤清晰**：复杂任务遵循"收集信息 → 制定方案 → 执行 → 验证结果 → 总结汇报"的流程，一步一步完成。
4. **及时执行**：写完脚本文件后，必须立即调用 execute_command 执行该脚本，不要只写不执行。
5. **避免重复**：如果文件已经创建且内容正确，不要重复写入相同内容，直接执行即可。
6. **有问必答**：所有工具执行完成后，必须给用户一段自然语言总结，说明执行结果和下一步建议，不要只输出工具结果就结束。
7. **简洁高效**：用户都是系统管理员，直接给出可执行的结果，不需要废话。
8. **安全第一**：删除、停止、重启、修改配置等危险操作，先说明影响再执行。

## 可用工具说明
- get_system_info: 获取系统基本信息
- list_processes: 查看进程列表
- execute_command: 执行 Shell 命令，危险命令会被拦截
- container_*: Docker 容器管理
- website_*: 网站/Nginx 配置管理
- database_*: 数据库管理
- firewall_*: 防火墙规则管理
- list_files / read_file / write_file: 文件操作
- backup_*: 备份管理
- web_search / web_fetch: 网络搜索
- get_nginx_logs: 查看 Nginx 日志

## 输出要求
- 默认使用中文回复
- 工具执行完成后一定要给出总结
- 如果任务失败，说明失败原因和解决建议`

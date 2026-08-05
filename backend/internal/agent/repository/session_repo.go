package repository

import (
	"encoding/json"
	"errors"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"gorm.io/gorm"
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

	// 修复中断导致的消息序列不完整：
	// 1. 如果 assistant 消息含 tool_calls，但后续缺少对应的 tool result，补充中断标记
	// 2. 如果存在孤立的 tool 消息（前面没有对应的 assistant(tool_calls)），删除并清理
	llmMessages = sm.normalizeMessageSequence(sessionID, llmMessages)

	return llmMessages, nil
}

// normalizeMessageSequence 把历史消息重建为对 LLM 合法的顺序：
//  1. 每条 assistant(tool_calls) 后立即补齐该消息内所有 tool_call_id 的响应
//     （有真实结果用真实结果，缺失时用内存占位，不写数据库）；
//  2. 数据库里位置错乱/重复的 tool 消息按“最后一个结果”收敛，不再原样透传。
//
// 这样即使发生过强制停止/确认中断，发送给模型的序列也不会出现
// “assistant(tool_calls) 后面没有紧跟 tool 响应”的 400 错误。
func (sm *SessionManager) normalizeMessageSequence(sessionID uint, messages []provider.LLMMessage) []provider.LLMMessage {
	toolByID := make(map[string]provider.LLMMessage)
	for _, m := range messages {
		if m.Role == "tool" && m.ToolCallID != "" {
			toolByID[m.ToolCallID] = m // 同一 ID 多条时取最后一条（真实结果通常在占位之后写入）
		}
	}

	consumed := make(map[string]bool)
	result := make([]provider.LLMMessage, 0, len(messages))
	for _, m := range messages {
		if m.Role == "tool" {
			// tool 消息统一由 assistant 分支按序输出，避免错位
			continue
		}
		result = append(result, m)
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if tc.ID == "" || consumed[tc.ID] {
					continue
				}
				consumed[tc.ID] = true
				if tm, ok := toolByID[tc.ID]; ok {
					result = append(result, tm)
				} else {
					global.LOG.Warnf("[Session] 会话%d: tool_call(id=%s) 缺少结果，使用内存占位", sessionID, tc.ID)
					result = append(result, provider.LLMMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    "[操作已中断：用户停止了回复]",
					})
				}
			}
		}
	}
	return result
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
	// 同一个 tool_call_id 只保留一条结果，避免占位/真实结果重复导致 LLM 400
	if err := global.DB.Where("session_id = ? AND tool_call_id = ? AND role = ?", sessionID, toolCallID, "tool").
		Delete(&model.AgentMessage{}).Error; err != nil {
		return err
	}
	return global.DB.Create(&model.AgentMessage{
		SessionID:  sessionID,
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
		ToolName:   toolName,
	}).Error
}

// DeleteLastAssistantMessage 删除会话中最后一条 assistant 消息（用于“重新生成”）
func (sm *SessionManager) DeleteLastAssistantMessage(sessionID uint) error {
	var last model.AgentMessage
	err := global.DB.Where("session_id = ? AND role = ?", sessionID, "assistant").
		Order("id DESC").First(&last).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return global.DB.Delete(&last).Error
}

func (sm *SessionManager) UpdateSessionTitle(sessionID uint, title string) {
	global.DB.Model(&model.AgentSession{}).Where("id = ?", sessionID).Update("title", title)
}

// CompressIfNeeded 已废弃：压缩逻辑已迁移到 engine.go 的 MicroCompressionStrategy。
// 保留此方法仅为向后兼容，直接返回原消息列表不做任何压缩。
// 新代码应使用 compression.MicroCompressionStrategy.ShouldCompress + Compress。
// Deprecated:
func (sm *SessionManager) CompressIfNeeded(messages []provider.LLMMessage) []provider.LLMMessage {
	return messages
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
3. **高效执行**：完成核心目标即可，不要做过多无关的验证、检查步骤，避免过度探索。
4. **及时执行**：写完脚本文件后，必须立即调用 execute_command 执行该脚本，不要只写不执行。
5. **避免重复**：如果文件已经创建且内容正确，不要重复写入相同内容，直接执行即可。不要重复调用相同工具相同参数。
6. **及时回复**：工具执行完成后，核心任务达成时立即总结回复用户，不要做多余的额外检查。所有操作结束后必须给用户一段自然语言总结。
7. **简洁高效**：用户都是系统管理员，直接给出可执行的结果，不需要废话。
8. **安全第一**：删除、停止、重启、修改配置等危险操作，先说明影响再执行。
9. **尽快收尾**：如果已经收集到足够信息完成任务，直接回复总结，不要继续调用更多工具。

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
- 任务完成或核心目标达成后，立即给出总结回复，不要继续调用工具做无关检查
- 如果任务失败，说明失败原因和解决建议
- 总结要清晰说明：完成了什么、结果如何、访问地址/账号信息等关键内容`

package repository

import (
	"encoding/json"
	"fmt"

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

	// 修复中断导致的消息序列不完整：
	// 1. 如果 assistant 消息含 tool_calls，但后续缺少对应的 tool result，补充中断标记
	// 2. 如果存在孤立的 tool 消息（前面没有对应的 assistant(tool_calls)），删除并清理
	llmMessages = sm.fixIncompleteToolCalls(sessionID, llmMessages)

	return llmMessages, nil
}

// fixIncompleteToolCalls 修复消息序列完整性。
// 处理两种问题：
//  1. assistant(tool_calls) 缺少对应的 tool result → 补充虚拟中断消息
//  2. 孤立的 tool 消息（前面没有 assistant(tool_calls)）→ 标记为内容（防止 LLM API 400）
//
// 两类问题都可能在多次中断/重试中积累，必须同时处理。
func (sm *SessionManager) fixIncompleteToolCalls(sessionID uint, messages []provider.LLMMessage) []provider.LLMMessage {
	if len(messages) == 0 {
		return messages
	}

	// 步骤 1：先清理孤立的 tool 消息（前面没有 assistant(tool_calls)）
	messages = sm.removeOrphanToolMessages(sessionID, messages)

	// 步骤 2：收集已有 tool_call_id → tool result 映射
	toolResults := make(map[string]bool)
	for _, m := range messages {
		if m.Role == "tool" && m.ToolCallID != "" {
			toolResults[m.ToolCallID] = true
		}
	}

	// 步骤 3：检查每条 assistant 消息的 tool_calls 是否都有对应 result
	var result []provider.LLMMessage
	for _, m := range messages {
		result = append(result, m)

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				if !toolResults[tc.ID] {
					global.LOG.Warnf("[Session] 检测到不完整的 tool_call(id=%s, name=%s)，补充中断标记",
						tc.ID, tc.Function.Name)
					result = append(result, provider.LLMMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    "[操作已中断：用户停止了回复]",
					})
					_ = sm.SaveToolResult(sessionID, tc.ID, tc.Function.Name, "[操作已中断：用户停止了回复]")
				}
			}
		}
	}

	return result
}

// removeOrphanToolMessages 清理孤立的 tool 消息。
// 触发场景：用户连续中断时，saveInterruptedToolResults 可能将 tool result 写入数据库
// 但 assistant(tool_calls) 消息保存失败（context canceled），导致数据库中只有 tool 没有 tool_calls。
// 此外，多次中断/重试可能产生重复的 tool result。
// 此函数将孤立的 tool 消息降级为 assistant 消息（用 `[上次结果已废弃]` 包装），
// 避免 LLM API 返回 400 "Messages with role 'tool' must be a response to a preceding message with 'tool_calls'"。
func (sm *SessionManager) removeOrphanToolMessages(sessionID uint, messages []provider.LLMMessage) []provider.LLMMessage {
	// 收集所有 assistant 消息中的 tool_call_id
	assistantToolCalls := make(map[string]bool)
	for _, m := range messages {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				assistantToolCalls[tc.ID] = true
			}
		}
	}

	// 检查每个 tool 消息是否有对应的 assistant(tool_calls)
	// 如果没有，降级为 assistant 消息
	modified := false
	result := make([]provider.LLMMessage, 0, len(messages))
	for _, m := range messages {
		if m.Role == "tool" {
			if m.ToolCallID == "" || !assistantToolCalls[m.ToolCallID] {
				global.LOG.Warnf("[Session] 检测到孤立的 tool 消息(tool_call_id=%s)，降级为 assistant 消息",
					m.ToolCallID)
				// 降级为 assistant 消息，保留原内容供 LLM 参考
				result = append(result, provider.LLMMessage{
					Role:    "assistant",
					Content: fmt.Sprintf("[前序操作结果已废弃] %s", m.Content),
				})
				// 同步修复数据库，将原 tool 消息标记为孤立（避免下次重复处理）
				// 注意：保留数据库原记录（不动），仅在内存中转换
				modified = true
				continue
			}
		}
		result = append(result, m)
	}

	_ = modified
	_ = sessionID
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

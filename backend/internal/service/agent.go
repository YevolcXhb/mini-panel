package service

import (
	"github.com/minipanel/minipanel/internal/agent/repository"
	"github.com/minipanel/minipanel/internal/model"
)

// AgentService Agent 业务层（仅配置和会话管理，不涉及 LLM 调用）
type AgentService struct {
	configRepo  *repository.ConfigRepo
	sessionRepo *repository.SessionManager
}

func NewAgentService() *AgentService {
	return &AgentService{
		configRepo:  repository.NewConfigRepo(),
		sessionRepo: repository.NewSessionManager(),
	}
}

// GetConfig 获取用户配置
func (s *AgentService) GetConfig(userID uint) (*model.AgentConfig, error) {
	return s.configRepo.GetOrCreate(userID)
}

// UpdateConfig 更新配置
func (s *AgentService) UpdateConfig(userID uint, cfg *model.AgentConfig) error {
	updates := map[string]interface{}{
		"provider":                 cfg.Provider,
		"base_url":                 cfg.BaseURL,
		"model":                    cfg.Model,
		"temperature":              cfg.Temperature,
		"max_tokens":               cfg.MaxTokens,
		"enabled":                  cfg.Enabled,
		"system_prompt":            cfg.SystemPrompt,
		"skills":                   cfg.Skills,
		"allow_dangerous_commands": cfg.AllowDangerousCommands,
		"exec_timeout_seconds":     cfg.ExecTimeoutSeconds,
	}
	if cfg.APIKey != "" {
		updates["api_key"] = cfg.APIKey
	}
	return s.configRepo.Update(userID, updates)
}

// UpdateAPIKey 单独更新 API Key（支持清空）
func (s *AgentService) UpdateAPIKey(userID uint, apiKey string) error {
	return s.configRepo.Update(userID, map[string]interface{}{"api_key": apiKey})
}

// CreateSession 创建会话
func (s *AgentService) CreateSession(userID uint, title string) (*model.AgentSession, error) {
	return s.sessionRepo.CreateSession(userID, title)
}

// ListSessions 列会话
func (s *AgentService) ListSessions(userID uint) ([]model.AgentSession, error) {
	return s.sessionRepo.ListSessions(userID)
}

// DeleteSession 删除会话
func (s *AgentService) DeleteSession(sessionID uint, userID uint) error {
	return s.sessionRepo.DeleteSession(sessionID, userID)
}

// GetSessionMessages 获取会话消息
func (s *AgentService) GetSessionMessages(sessionID uint, userID uint) ([]model.AgentMessage, error) {
	session, err := s.sessionRepo.GetSession(sessionID, userID)
	if err != nil {
		return nil, err
	}
	return session.Messages, nil
}

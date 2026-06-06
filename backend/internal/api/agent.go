package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-contrib/sse"
	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/agent"
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/repository"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/service"
)

type AgentAPI struct {
	service *service.AgentService
}

func NewAgentAPI() *AgentAPI {
	return &AgentAPI{service: service.NewAgentService()}
}

// RegisterRoutes 注册路由
func (a *AgentAPI) RegisterRoutes(r *gin.RouterGroup) {
	agentGroup := r.Group("/agent")
	{
		agentGroup.GET("/config", a.GetConfig)
		agentGroup.PUT("/config", a.UpdateConfig)
		agentGroup.GET("/sessions", a.ListSessions)
		agentGroup.POST("/sessions", a.CreateSession)
		agentGroup.DELETE("/sessions/:id", a.DeleteSession)
		agentGroup.GET("/sessions/:id/messages", a.GetSessionMessages)
		agentGroup.POST("/chat", a.Chat)
		agentGroup.POST("/confirm", a.Confirm)
	}
}

func (a *AgentAPI) getUserID(c *gin.Context) uint {
	uid, _ := c.Get("userID")
	if id, ok := uid.(uint); ok {
		return id
	}
	if id, ok := uid.(float64); ok {
		return uint(id)
	}
	return 0
}

// GetConfig 获取配置
func (a *AgentAPI) GetConfig(c *gin.Context) {
	userID := a.getUserID(c)
	cfg, err := a.service.GetConfig(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"provider":      cfg.Provider,
			"base_url":      cfg.BaseURL,
			"model":         cfg.Model,
			"temperature":   cfg.Temperature,
			"max_tokens":    cfg.MaxTokens,
			"enabled":       cfg.Enabled,
			"system_prompt": cfg.SystemPrompt,
			"skills":        cfg.Skills,
		},
		"available_skills": agent.GetAllSkills(),
	})
}

// UpdateConfig 更新配置
func (a *AgentAPI) UpdateConfig(c *gin.Context) {
	userID := a.getUserID(c)
	var req model.AgentConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := a.service.UpdateConfig(userID, &req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "配置已更新"})
}

// ListSessions 列会话
func (a *AgentAPI) ListSessions(c *gin.Context) {
	userID := a.getUserID(c)
	sessions, err := a.service.ListSessions(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": sessions})
}

// CreateSession 创建会话
func (a *AgentAPI) CreateSession(c *gin.Context) {
	userID := a.getUserID(c)
	var req struct {
		Title string `json:"title"`
	}
	c.ShouldBindJSON(&req)
	if req.Title == "" {
		req.Title = "新会话"
	}
	session, err := a.service.CreateSession(userID, req.Title)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": session})
}

// DeleteSession 删除会话
func (a *AgentAPI) DeleteSession(c *gin.Context) {
	userID := a.getUserID(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := a.service.DeleteSession(uint(id), userID); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "已删除"})
}

// GetSessionMessages 获取消息
func (a *AgentAPI) GetSessionMessages(c *gin.Context) {
	userID := a.getUserID(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	messages, err := a.service.GetSessionMessages(uint(id), userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": messages})
}

// Chat 流式聊天（SSE）
func (a *AgentAPI) Chat(c *gin.Context) {
	userID := a.getUserID(c)
	var req struct {
		SessionID uint   `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if req.Message == "" {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "消息不能为空"})
		return
	}

	// 获取配置
	cfg, err := a.service.GetConfig(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "获取配置失败: " + err.Error()})
		return
	}
	if !cfg.Enabled {
		c.JSON(http.StatusOK, gin.H{"code": 403, "message": "Agent 未启用"})
		return
	}

	// 更新会话标题
	sm := repository.NewSessionManager()
	session, _ := sm.GetSession(req.SessionID, userID)
	if session != nil && session.Title == "新会话" {
		sm.UpdateSessionTitle(req.SessionID, agent.BuildTitleFromContent(req.Message))
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	stream := make(chan agent.StreamChunk, 10)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 防止 goroutine panic 导致整个进程崩溃
			}
			close(stream)
		}()
		p, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model)
		if err != nil {
			stream <- agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()}
			return
		}
		var skillIDs []string
		_ = json.Unmarshal([]byte(cfg.Skills), &skillIDs)
		engine := agent.NewEngineWithProvider(p, skillIDs)
		_ = engine.Run(ctx, req.SessionID, req.Message, stream)
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case chunk, ok := <-stream:
			if !ok {
				return false
			}
			c.Render(-1, sse.Event{Event: "message", Data: chunk})
			return true
		case <-ctx.Done():
			return false
		}
	})
}

// Confirm 确认执行
func (a *AgentAPI) Confirm(c *gin.Context) {
	userID := a.getUserID(c)
	var req struct {
		SessionID  uint   `json:"session_id"`
		ToolCallID string `json:"tool_call_id"`
		Confirmed  bool   `json:"confirmed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}

	cfg, err := a.service.GetConfig(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "获取配置失败: " + err.Error()})
		return
	}
	if !cfg.Enabled {
		c.JSON(http.StatusOK, gin.H{"code": 403, "message": "Agent 未启用"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	stream := make(chan agent.StreamChunk, 10)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 防止 goroutine panic 导致整个进程崩溃
			}
			close(stream)
		}()
		p, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model)
		if err != nil {
			stream <- agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()}
			return
		}
		var skillIDs []string
		_ = json.Unmarshal([]byte(cfg.Skills), &skillIDs)
		engine := agent.NewEngineWithProvider(p, skillIDs)
		_ = engine.RunWithConfirm(ctx, req.SessionID, req.ToolCallID, req.Confirmed, stream)
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case chunk, ok := <-stream:
			if !ok {
				return false
			}
			c.Render(-1, sse.Event{Event: "message", Data: chunk})
			return true
		case <-ctx.Done():
			return false
		}
	})
}

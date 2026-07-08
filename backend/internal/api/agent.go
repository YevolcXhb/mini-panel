package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-contrib/sse"
	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/agent"
	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/agent/repository"
	"github.com/minipanel/minipanel/internal/agent/tools"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/service"
)

type AgentAPI struct {
	service *service.AgentService

	// orchestrators 管理 sessionID → Orchestrator 的映射。
	// Orchestrate 时创建并存入，ConfirmPlan 时取出并删除。
	// 30分钟未确认的计划由 cleanupOrchestrators 自动清理。
	orchMu        sync.Mutex
	orchestrators map[uint]*orchEntry
}

// orchEntry 编排器条目，记录创建时间用于超时清理
type orchEntry struct {
	orch      *agent.Orchestrator
	createdAt time.Time
}

// orchCleanupInterval 编排器超时清理间隔
const orchCleanupInterval = 10 * time.Minute

// orchTimeout 编排器等待确认的超时时间
const orchTimeout = 30 * time.Minute

func NewAgentAPI() *AgentAPI {
	a := &AgentAPI{
		service:       service.NewAgentService(),
		orchestrators: make(map[uint]*orchEntry),
	}
	go a.cleanupOrchestrators()
	return a
}

// cleanupOrchestrators 定期清理超时的待确认编排器，防止内存泄漏
func (a *AgentAPI) cleanupOrchestrators() {
	ticker := time.NewTicker(orchCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		a.orchMu.Lock()
		now := time.Now()
		for sid, entry := range a.orchestrators {
			if now.Sub(entry.createdAt) > orchTimeout {
				global.LOG.Infof("[AgentAPI] 清理超时编排器: sessionID=%d", sid)
				delete(a.orchestrators, sid)
			}
		}
		a.orchMu.Unlock()
	}
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
		agentGroup.POST("/orchestrate", a.Orchestrate)
		agentGroup.POST("/confirm-plan", a.ConfirmPlan)
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

// applyExecOptionsFromConfig 根据用户配置设置 ExecTool 的全局运行时配置。
// 必须在 Chat/Confirm/Orchestrate/ConfirmPlan 入口处、GetConfig 之后调用。
func (a *AgentAPI) applyExecOptionsFromConfig(cfg *model.AgentConfig) {
	timeout := time.Duration(cfg.ExecTimeoutSeconds) * time.Second
	if cfg.ExecTimeoutSeconds <= 0 {
		timeout = 120 * time.Second
	}
	tools.SetExecOptions(tools.ExecOptions{
		AllowDangerous: cfg.AllowDangerousCommands,
		Timeout:        timeout,
	})
	global.LOG.Infof("[AgentAPI] 已应用 ExecOptions: AllowDangerous=%v, Timeout=%v", cfg.AllowDangerousCommands, timeout)
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
	a.applyExecOptionsFromConfig(cfg)

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
		p, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model, float32(cfg.Temperature), cfg.MaxTokens)
		if err != nil {
			stream <- agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()}
			return
		}
		var skillIDs []string
		_ = json.Unmarshal([]byte(cfg.Skills), &skillIDs)
		engine := agent.NewEngineWithProvider(p, skillIDs, cfg.SystemPrompt)
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
	a.applyExecOptionsFromConfig(cfg)

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
		p, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model, float32(cfg.Temperature), cfg.MaxTokens)
		if err != nil {
			stream <- agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()}
			return
		}
		var skillIDs []string
		_ = json.Unmarshal([]byte(cfg.Skills), &skillIDs)
		engine := agent.NewEngineWithProvider(p, skillIDs, cfg.SystemPrompt)
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

// Orchestrate 触发三阶段编排（PLANNING → plan_ready → 等待确认）
// 用户确认后通过 ConfirmPlan 端点继续 CODING → REVIEWING。
func (a *AgentAPI) Orchestrate(c *gin.Context) {
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

	cfg, err := a.service.GetConfig(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": "获取配置失败: " + err.Error()})
		return
	}
	if !cfg.Enabled {
		c.JSON(http.StatusOK, gin.H{"code": 403, "message": "Agent 未启用"})
		return
	}
	a.applyExecOptionsFromConfig(cfg)

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
				global.LOG.Errorf("[AgentAPI] Orchestrate panic: %v", r)
			}
			close(stream)
		}()
		p, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model, float32(cfg.Temperature), cfg.MaxTokens)
		if err != nil {
			stream <- agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()}
			return
		}
		var skillIDs []string
		_ = json.Unmarshal([]byte(cfg.Skills), &skillIDs)
		_, orchestrator := agent.NewOrchestratorWithProvider(p, skillIDs, cfg.SystemPrompt)

		// 存入 map 供后续 ConfirmPlan 使用
		a.orchMu.Lock()
		a.orchestrators[req.SessionID] = &orchEntry{
			orch:      orchestrator,
			createdAt: time.Now(),
		}
		a.orchMu.Unlock()
		global.LOG.Infof("[AgentAPI] 编排器已创建: sessionID=%d", req.SessionID)

		_ = orchestrator.Orchestrate(ctx, req.SessionID, req.Message, stream)
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

// ConfirmPlan 确认或取消编排计划。
// confirmed=true 时继续 CODING → REVIEWING；confirmed=false 时取消执行。
func (a *AgentAPI) ConfirmPlan(c *gin.Context) {
	userID := a.getUserID(c)
	var req struct {
		SessionID uint `json:"session_id"`
		Confirmed bool `json:"confirmed"`
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
	a.applyExecOptionsFromConfig(cfg)

	// 从 map 中取出编排器（取出后删除，ConfirmPlan 只能调用一次）
	a.orchMu.Lock()
	entry := a.orchestrators[req.SessionID]
	delete(a.orchestrators, req.SessionID)
	a.orchMu.Unlock()

	if entry == nil {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "找不到待确认的计划，可能已超时或未发起编排"})
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
				global.LOG.Errorf("[AgentAPI] ConfirmPlan panic: %v", r)
			}
			close(stream)
		}()
		_ = entry.orch.ConfirmPlan(ctx, req.SessionID, req.Confirmed, stream)
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

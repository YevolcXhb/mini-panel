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
	"github.com/minipanel/minipanel/internal/agent/agentcore"
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

	// 同一会话同一时间只允许一个运行中的 Chat/Confirm/Orchestrate/ConfirmPlan
	runMu   sync.Mutex
	running map[uint]bool
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

// sendChunk 安全地向 SSE 流发送数据：客户端断开（ctx 取消）时直接放弃本次发送
func sendChunk(ctx context.Context, stream chan<- agent.StreamChunk, chunk agent.StreamChunk) {
	select {
	case <-ctx.Done():
	case stream <- chunk:
	}
}

// adaptStream 把 agentcore 的 StreamChunk 适配到 agent.StreamChunk。
//
// 两个结构字段与 JSON tag 完全一致，所以只需逐字段搬运；
// 用独立 goroutine 转发是为了让上游 API 层的读出逻辑（c.Stream）保持原样，
// 前端 SSE 协议一个字节都不用改。
func adaptStream(src <-chan agentcore.StreamChunk) chan agent.StreamChunk {
	dst := make(chan agent.StreamChunk, 10)
	go func() {
		defer close(dst)
		for c := range src {
			out := agent.StreamChunk{
				Type:        c.Type,
				Content:     c.Content,
				ToolCallID:  c.ToolCallID,
				ToolName:    c.ToolName,
				Command:     c.Command,
				Message:     c.Message,
				Error:       c.Error,
				Success:     c.Success,
				Cached:      c.Cached,
				Phase:       c.Phase,
				TokensSaved: c.TokensSaved,
				StepNumber:  c.StepNumber,
				MaxSteps:    c.MaxSteps,
			}
			if c.Usage != nil {
				out.Usage = &agent.StreamUsage{
					Tokens:        c.Usage.Tokens,
					Threshold:     c.Usage.Threshold,
					ContextWindow: c.Usage.ContextWindow,
					Percent:       c.Usage.Percent,
					Compactions:   c.Usage.Compactions,
				}
			}
			dst <- out
		}
	}()
	return dst
}

func NewAgentAPI() *AgentAPI {
	a := &AgentAPI{
		service:       service.NewAgentService(),
		orchestrators: make(map[uint]*orchEntry),
		running:       make(map[uint]bool),
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

// RegisterRoutes 注册路由。r 应为已带 /agent 前缀并挂好权限中间件的分组。
func (a *AgentAPI) RegisterRoutes(r *gin.RouterGroup) {
	{
		r.GET("/config", a.GetConfig)
		r.PUT("/config", a.UpdateConfig)
		r.GET("/sessions", a.ListSessions)
		r.POST("/sessions", a.CreateSession)
		r.DELETE("/sessions/:id", a.DeleteSession)
		r.GET("/sessions/:id/messages", a.GetSessionMessages)
		r.GET("/sessions/:id/inbox", a.GetInbox)
		r.POST("/chat", a.Chat)
		r.POST("/confirm", a.Confirm)
		r.POST("/orchestrate", a.Orchestrate)
		r.POST("/confirm-plan", a.ConfirmPlan)
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
func (a *AgentAPI) execOptionsFromConfig(cfg *model.AgentConfig) tools.ExecOptions {
	timeout := time.Duration(cfg.ExecTimeoutSeconds) * time.Second
	if cfg.ExecTimeoutSeconds <= 0 {
		timeout = 120 * time.Second
	}
	opts := tools.ExecOptions{
		AllowDangerous: cfg.AllowDangerousCommands,
		Timeout:        timeout,
	}
	global.LOG.Infof("[AgentAPI] ExecOptions: AllowDangerous=%v, Timeout=%v", cfg.AllowDangerousCommands, timeout)
	return opts
}

// withExecOptions 把本次请求的执行配置注入 context，避免全局状态在并发会话间串扰
func (a *AgentAPI) withExecOptions(ctx context.Context, cfg *model.AgentConfig) context.Context {
	return tools.WithExecOptions(ctx, a.execOptionsFromConfig(cfg))
}

// beginRun 标记会话进入运行状态；已在运行时返回 false
func (a *AgentAPI) beginRun(sessionID uint) bool {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.running[sessionID] {
		return false
	}
	a.running[sessionID] = true
	return true
}

// endRun 释放会话运行锁
func (a *AgentAPI) endRun(sessionID uint) {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	delete(a.running, sessionID)
}

// isRunning 查询会话是否正在运行
func (a *AgentAPI) isRunning(sessionID uint) bool {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	return a.running[sessionID]
}

// validateSession 校验会话存在且属于当前用户；失败时直接响应并返回 false
func (a *AgentAPI) validateSession(c *gin.Context, userID uint, sessionID uint) bool {
	sm := repository.NewSessionManager()
	session, err := sm.GetSession(sessionID, userID)
	if err != nil || session == nil {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "会话不存在或无权访问"})
		return false
	}
	return true
}

// getOrchestrator 取当前会话对应的编排器（确认工具调用时判断是否处于编排模式）
func (a *AgentAPI) getOrchestrator(sessionID uint) *agent.Orchestrator {
	a.orchMu.Lock()
	defer a.orchMu.Unlock()
	if entry := a.orchestrators[sessionID]; entry != nil {
		return entry.orch
	}
	return nil
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
			"context_window":           cfg.ContextWindow,
			"max_steps":                cfg.MaxSteps,
			"use_orchestrator":         cfg.UseOrchestrator,
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
	// 上下文窗口按 0 表示"未指定"，交给下游用保守默认值，
	// 而不是把 0 写进预算里导致每轮都触发压缩。
	if req.ContextWindow < 0 {
		req.ContextWindow = 0
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
	// 会话删除后它的插话队列没有意义了，顺手释放，避免长期堆积
	agentcore.DefaultInboxes().Drop(uint(id))
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

// GetInbox 查询会话的运行中插话队列状态。
//
// 前端在 agent 运行期间可以轮询这个接口，显示"还有 N 条插话待注入"。
func (a *AgentAPI) GetInbox(c *gin.Context) {
	userID := a.getUserID(c)
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	sessionID := uint(id)
	if !a.validateSession(c, userID, sessionID) {
		return
	}
	inbox := agentcore.DefaultInboxes().Lookup(sessionID)
	if inbox == nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 200,
			"data": gin.H{"pending": 0, "total": 0, "running": a.isRunning(sessionID)},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"pending": inbox.Pending(),
			"total":   inbox.Total(),
			"running": a.isRunning(sessionID),
		},
	})
}

// Chat 流式聊天（SSE）
func (a *AgentAPI) Chat(c *gin.Context) {
	userID := a.getUserID(c)
	var req struct {
		SessionID  uint   `json:"session_id"`
		Message    string `json:"message"`
		Regenerate bool   `json:"regenerate"`
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
	if !a.validateSession(c, userID, req.SessionID) {
		return
	}
	if !a.beginRun(req.SessionID) {
		// 会话正在运行：不再直接拒绝，而是把这条消息投进 inbox，
		// 由引擎在下一个步边界注入上下文。
		// 这样用户在 agent 跑动中能直接补充信息、修正目标，
		// 不必等本轮结束再重发一遍。
		inbox := agentcore.DefaultInboxes().Get(req.SessionID)
		if inbox.Push(req.Message) {
			_ = repository.NewSessionManager().SaveUserMessage(req.SessionID, req.Message)
			global.LOG.Infof("[AgentAPI] 会话%d 正在运行，插话已入队（待注入 %d 条）",
				req.SessionID, inbox.Pending())
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": "插话已排队，将在当前步骤结束后注入",
				"data":    gin.H{"queued": true, "pending": inbox.Pending()},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "消息不能为空"})
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
	ctx = a.withExecOptions(ctx, cfg)

	go func() {
		defer a.endRun(req.SessionID)
		defer func() {
			if r := recover(); r != nil {
				// 防止 goroutine panic 导致整个进程崩溃
			}
			close(stream)
		}()
		p, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model, float32(cfg.Temperature), cfg.MaxTokens)
		if err != nil {
			sendChunk(ctx, stream, agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()})
			return
		}
		var skillIDs []string
		_ = json.Unmarshal([]byte(cfg.Skills), &skillIDs)
		// 纯 agent 模式：走 agentcore（DSH 风格 ReAct 主循环 + 事务式上下文压缩）。
		// 三阶段编排仅在显式开启 use_orchestrator 时使用。
		var engine *agentcore.Engine
		if cfg.UseOrchestrator {
			legacy := agent.NewEngineWithProvider(p, skillIDs, cfg.SystemPrompt)
			if req.Regenerate {
				_ = legacy.RunRegenerate(ctx, req.SessionID, stream)
			} else {
				_ = legacy.Run(ctx, req.SessionID, req.Message, stream)
			}
			return
		}
		engine, err = agentcore.NewFromConfig(cfg, skillIDs)
		if err != nil {
			sendChunk(ctx, stream, agent.StreamChunk{Type: "error", Error: "创建 Agent 失败: " + err.Error()})
			return
		}
		acStream := make(chan agentcore.StreamChunk, 10)
		go func() {
			for c := range adaptStream(acStream) {
				sendChunk(ctx, stream, c)
			}
		}()
		if req.Regenerate {
			_ = engine.RunRegenerate(ctx, req.SessionID, acStream)
		} else {
			_ = engine.Run(ctx, req.SessionID, req.Message, acStream)
		}
		close(acStream)
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
	if !a.validateSession(c, userID, req.SessionID) {
		return
	}
	if !a.beginRun(req.SessionID) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "该会话正在处理中，请稍后再试"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	stream := make(chan agent.StreamChunk, 10)
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	ctx = a.withExecOptions(ctx, cfg)

	go func() {
		defer a.endRun(req.SessionID)
		defer func() {
			if r := recover(); r != nil {
				// 防止 goroutine panic 导致整个进程崩溃
			}
			// 编排模式确认完成后，若整个编排已结束则清理编排器
			a.orchMu.Lock()
			if orch := a.orchestrators[req.SessionID]; orch != nil &&
				!orch.orch.HasPendingToolConfirm(req.SessionID) &&
				!orch.orch.HasPendingContinuation(req.SessionID) {
				delete(a.orchestrators, req.SessionID)
			}
			a.orchMu.Unlock()
			close(stream)
		}()
		// 编排模式下工具确认必须回到编排器继续对应 phase，否则会降级成普通对话
		if orch := a.getOrchestrator(req.SessionID); orch != nil && orch.HasPendingToolConfirm(req.SessionID) {
			_ = orch.ConfirmTool(ctx, req.SessionID, req.ToolCallID, req.Confirmed, stream)
			return
		}
		if _, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model, float32(cfg.Temperature), cfg.MaxTokens); err != nil {
			sendChunk(ctx, stream, agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()})
			return
		}
		var skillIDs []string
		_ = json.Unmarshal([]byte(cfg.Skills), &skillIDs)
		engine, err := agentcore.NewFromConfig(cfg, skillIDs)
		if err != nil {
			sendChunk(ctx, stream, agent.StreamChunk{Type: "error", Error: "创建 Agent 失败: " + err.Error()})
			return
		}
		acStream := make(chan agentcore.StreamChunk, 10)
		go func() {
			for c := range adaptStream(acStream) {
				sendChunk(ctx, stream, c)
			}
		}()
		_ = engine.RunWithConfirm(ctx, req.SessionID, req.ToolCallID, req.Confirmed, acStream)
		close(acStream)
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
	if !a.validateSession(c, userID, req.SessionID) {
		return
	}
	if !a.beginRun(req.SessionID) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "该会话正在处理中，请稍后再试"})
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
	ctx = a.withExecOptions(ctx, cfg)

	go func() {
		defer a.endRun(req.SessionID)
		defer func() {
			if r := recover(); r != nil {
				global.LOG.Errorf("[AgentAPI] Orchestrate panic: %v", r)
			}
			close(stream)
		}()
		p, err := provider.NewProvider(cfg.Provider, cfg.BaseURL, cfg.APIKey, cfg.Model, float32(cfg.Temperature), cfg.MaxTokens)
		if err != nil {
			sendChunk(ctx, stream, agent.StreamChunk{Type: "error", Error: "创建 Provider 失败: " + err.Error()})
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
	if !a.validateSession(c, userID, req.SessionID) {
		return
	}
	if !a.beginRun(req.SessionID) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "该会话正在处理中，请稍后再试"})
		return
	}

	// 取出编排器但保留在 map 中：CODING/REVIEWING 阶段的工具确认还需要它，
	// 编排全部结束后才由 goroutine 清理
	a.orchMu.Lock()
	entry := a.orchestrators[req.SessionID]
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
	ctx = a.withExecOptions(ctx, cfg)

	go func() {
		defer a.endRun(req.SessionID)
		defer func() {
			if r := recover(); r != nil {
				global.LOG.Errorf("[AgentAPI] ConfirmPlan panic: %v", r)
			}
			a.orchMu.Lock()
			if !entry.orch.HasPendingToolConfirm(req.SessionID) && !entry.orch.HasPendingContinuation(req.SessionID) {
				delete(a.orchestrators, req.SessionID)
			}
			a.orchMu.Unlock()
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

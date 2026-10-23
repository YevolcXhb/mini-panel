package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/service"
)

type FirewallAPI struct {
	service *service.FirewallService
}

func NewFirewallAPI() *FirewallAPI {
	return &FirewallAPI{service: service.NewFirewallService()}
}

func (h *FirewallAPI) Create(c *gin.Context) {
	var req model.FirewallRule
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := h.service.Create(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Firewall rule created"})
}

func (h *FirewallAPI) List(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// ListDeletedRules 列出已软删除的规则（回收站）
func (h *FirewallAPI) ListDeletedRules(c *gin.Context) {
	items, err := h.service.ListDeletedRules()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": items})
}

// RestoreRule 恢复被软删除的规则
func (h *FirewallAPI) RestoreRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	if err := h.service.RestoreRule(uint(id)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Firewall rule restored"})
}

// ClearDeletedRules 一键清空已删除规则（回收站）
func (h *FirewallAPI) ClearDeletedRules(c *gin.Context) {
	n, err := h.service.ClearDeletedRules()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": fmt.Sprintf("已清空 %d 条已删除规则", n), "data": n})
}

func (h *FirewallAPI) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	var req model.FirewallRule
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	req.ID = uint(id)
	if err := h.service.Update(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Firewall rule updated"})
}

func (h *FirewallAPI) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	if err := h.service.Delete(uint(id)); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Firewall rule deleted"})
}

func (h *FirewallAPI) Apply(c *gin.Context) {
	msg, err := h.service.ApplyRules()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": msg})
}

func (h *FirewallAPI) Status(c *gin.Context) {
	status, err := h.service.GetStatus()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": status})
}

func (h *FirewallAPI) Start(c *gin.Context) {
	if err := h.service.Start(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Firewall started"})
}

func (h *FirewallAPI) Stop(c *gin.Context) {
	if err := h.service.Stop(); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Firewall stopped"})
}

// Diagnose 一键诊断防火墙环境
func (h *FirewallAPI) Diagnose(c *gin.Context) {
	report := h.service.Diagnose()
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": report})
}

// LiveRules 实时查看系统规则
// iptables 后端返回文本；nft 后端返回结构化 JSON
func (h *FirewallAPI) LiveRules(c *gin.Context) {
	chain := c.Query("chain")
	family := c.Query("family")
	table := c.Query("table")

	// struct=1 或无参数时优先给结构化数据，前端不再解析文本
	if c.Query("struct") == "1" {
		view, err := h.service.SystemRules(table, chain)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 200, "data": view})
		return
	}

	output, err := h.service.LiveRules(chain, family, table)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": output})
}

// SystemRules 返回结构化的内核规则视图（目前仅 nft 后端支持）
func (h *FirewallAPI) SystemRules(c *gin.Context) {
	table := c.Query("table")
	chain := c.Query("chain")
	view, err := h.service.SystemRules(table, chain)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": view})
}

// InsertRule 插入规则到指定位置
func (h *FirewallAPI) InsertRule(c *gin.Context) {
	var req struct {
		Chain    string   `json:"chain"`
		Position int      `json:"position"`
		Spec     []string `json:"spec"`
		Family   string   `json:"family"`
		Table    string   `json:"table"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if err := h.service.InsertRule(req.Chain, req.Position, req.Spec, req.Family, req.Table); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "规则已插入"})
}

// DeleteLiveRule 删除系统规则（nft 后端按 handle）
func (h *FirewallAPI) DeleteLiveRule(c *gin.Context) {
	chain := c.Query("chain")
	family := c.Query("family")
	table := c.Query("table")
	numStr := c.Query("num")
	if numStr == "" {
		numStr = c.Query("handle")
	}
	num, err := strconv.Atoi(numStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "无效的行号/handle"})
		return
	}
	if err := h.service.DeleteLiveRule(chain, num, family, table); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "规则已删除"})
}

// Lockdown 一键内网-only 模式
func (h *FirewallAPI) Lockdown(c *gin.Context) {
	msg, err := h.service.Lockdown()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": msg})
}

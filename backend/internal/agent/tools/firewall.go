package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/service"
)

// FirewallListTool 列出防火墙规则
type FirewallListTool struct{}

func NewFirewallListTool() *FirewallListTool { return &FirewallListTool{} }

func (t *FirewallListTool) Name() string                     { return "list_firewall_rules" }
func (t *FirewallListTool) Description() string              { return "列出所有防火墙规则。" }
func (t *FirewallListTool) Parameters() []provider.ToolParam { return nil }
func (t *FirewallListTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewFirewallService()
	rules, err := svc.List()
	if err != nil {
		return ErrorErr(err)
	}
	if len(rules) == 0 {
		return SuccessResult("没有防火墙规则")
	}
	var sb strings.Builder
	for _, r := range rules {
		enabled := "已启用"
		if !r.Enabled {
			enabled = "已禁用"
		}
		sb.WriteString(fmt.Sprintf("ID: %d | 名称: %s | 方向: %s | 类型: %s | 动作: %s | 协议: %s | 端口: %s | IP: %s | 状态: %s | 备注: %s\n",
			r.ID, r.Name, r.Direction, r.Type, r.Action, r.Protocol, r.Port, r.IP, enabled, r.Note))
	}
	return SuccessResult(sb.String())
}

// FirewallOpTool 防火墙操作
type FirewallOpTool struct{}

func NewFirewallOpTool() *FirewallOpTool { return &FirewallOpTool{} }

func (t *FirewallOpTool) Name() string { return "firewall_operation" }
func (t *FirewallOpTool) Description() string {
	return "防火墙操作: apply(应用规则)/delete(删除规则)。新增规则建议通过面板操作。"
}
func (t *FirewallOpTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "action", Type: "string", Description: "操作: apply/delete", Required: true},
		{Name: "id", Type: "integer", Description: "规则 ID (delete 时必填)", Required: false},
	}
}
func (t *FirewallOpTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewFirewallService()
	action := GetString(args, "action")
	switch action {
	case "apply":
		msg, err := svc.ApplyRules()
		if err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(msg)
	case "delete":
		id := uint(GetInt(args, "id"))
		if err := svc.Delete(id); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("防火墙规则 %d 已删除", id))
	default:
		return ErrorResult("不支持的操作: %s", action)
	}
}

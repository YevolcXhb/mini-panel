package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/service"
)

// DatabaseListTool 列出数据库
type DatabaseListTool struct{}

func (t *DatabaseListTool) Name() string                     { return "list_databases" }
func (t *DatabaseListTool) Description() string              { return "列出所有数据库实例。" }
func (t *DatabaseListTool) Parameters() []provider.ToolParam { return nil }
func (t *DatabaseListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	svc := service.NewDatabaseService()
	dbs, err := svc.List()
	if err != nil {
		return "", err
	}
	if len(dbs) == 0 {
		return "没有数据库实例", nil
	}
	var sb strings.Builder
	for _, d := range dbs {
		enabled := "已启用"
		if !d.Enabled {
			enabled = "已禁用"
		}
		sb.WriteString(fmt.Sprintf("ID: %d | 名称: %s | 类型: %s | 地址: %s:%d | 状态: %s\n",
			d.ID, d.Name, d.Type, d.Host, d.Port, enabled))
	}
	return sb.String(), nil
}

// DatabaseOpTool 数据库操作
type DatabaseOpTool struct{}

func (t *DatabaseOpTool) Name() string { return "database_op" }
func (t *DatabaseOpTool) Description() string {
	return "对数据库执行操作: test(测试连接)/delete(删除实例)。"
}
func (t *DatabaseOpTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "id", Type: "integer", Description: "数据库实例 ID", Required: true},
		{Name: "action", Type: "string", Description: "操作: test/delete", Required: true},
	}
}
func (t *DatabaseOpTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	svc := service.NewDatabaseService()
	id := uint(GetInt(args, "id"))
	action := GetString(args, "action")
	switch action {
	case "test":
		db, err := svc.GetByID(id)
		if err != nil {
			return fmt.Sprintf("获取数据库失败: %v", err), nil
		}
		msg, err := svc.TestConnection(db)
		if err != nil {
			return fmt.Sprintf("连接测试失败: %v", err), nil
		}
		return fmt.Sprintf("连接测试成功: %s", msg), nil
	case "delete":
		return "", svc.Delete(id)
	default:
		return "", fmt.Errorf("不支持的操作: %s", action)
	}
}

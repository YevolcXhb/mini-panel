package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/service"
)

// ContainerListTool 列出容器
type ContainerListTool struct{}

func NewContainerListTool() *ContainerListTool { return &ContainerListTool{} }

func (t *ContainerListTool) Name() string        { return "list_containers" }
func (t *ContainerListTool) Description() string { return "列出所有 Docker 容器及其状态。" }
func (t *ContainerListTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "status", Type: "string", Description: "过滤状态: running/stopped/all", Required: false},
	}
}
func (t *ContainerListTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewContainerService()
	containers, err := svc.List()
	if err != nil {
		return ErrorErr(err)
	}
	if len(containers) == 0 {
		return SuccessResult("没有容器")
	}
	statusFilter := GetString(args, "status")
	var sb strings.Builder
	count := 0
	for _, c := range containers {
		if statusFilter != "" && statusFilter != "all" && c.Status != statusFilter {
			continue
		}
		count++
		sb.WriteString(fmt.Sprintf("名称: %s | 镜像: %s | 状态: %s\n",
			c.Name, c.Image, c.Status))
	}
	return SuccessResult(fmt.Sprintf("共 %d 个容器:\n%s", count, sb.String()))
}

// ContainerOpTool 容器操作
type ContainerOpTool struct{}

func NewContainerOpTool() *ContainerOpTool { return &ContainerOpTool{} }

func (t *ContainerOpTool) Name() string { return "container_op" }
func (t *ContainerOpTool) Description() string {
	return "对指定容器执行操作: start/stop/restart/remove。"
}
func (t *ContainerOpTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "name", Type: "string", Description: "容器名称或 ID", Required: true},
		{Name: "action", Type: "string", Description: "操作: start/stop/restart/remove", Required: true},
	}
}
func (t *ContainerOpTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewContainerService()
	name := GetString(args, "name")
	action := GetString(args, "action")
	switch action {
	case "start":
		if err := svc.Start(name); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("容器 %s 已启动", name))
	case "stop":
		if err := svc.Stop(name); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("容器 %s 已停止", name))
	case "restart":
		if err := svc.Stop(name); err != nil {
			return ErrorErr(err)
		}
		if err := svc.Start(name); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("容器 %s 已重启", name))
	case "remove":
		if err := svc.Remove(name); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("容器 %s 已删除", name))
	default:
		return ErrorResult("不支持的操作: %s", action)
	}
}

// ContainerLogsTool 容器日志
type ContainerLogsTool struct{}

func NewContainerLogsTool() *ContainerLogsTool { return &ContainerLogsTool{} }

func (t *ContainerLogsTool) Name() string        { return "get_container_logs" }
func (t *ContainerLogsTool) Description() string { return "获取指定容器的最近日志。" }
func (t *ContainerLogsTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "name", Type: "string", Description: "容器名称或 ID", Required: true},
		{Name: "tail", Type: "integer", Description: "日志行数，默认 50", Required: false},
	}
}
func (t *ContainerLogsTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewContainerService()
	name := GetString(args, "name")
	tail := GetInt(args, "tail")
	if tail <= 0 {
		tail = 50
	}
	logs, err := svc.Logs(name, tail)
	if err != nil {
		return ErrorErr(err)
	}
	return SuccessResult(fmt.Sprintf("容器 %s 最近 %d 行日志:\n%s", name, tail, logs))
}

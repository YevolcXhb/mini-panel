package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/service"
)

// BackupListTool 列出备份任务和记录
type BackupListTool struct{}

func NewBackupListTool() *BackupListTool { return &BackupListTool{} }

func (t *BackupListTool) Name() string        { return "list_backups" }
func (t *BackupListTool) Description() string { return "列出备份任务和最近的备份记录。" }
func (t *BackupListTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "type", Type: "string", Description: "列表类型: tasks/records", Required: true},
		{Name: "task_id", Type: "integer", Description: "任务ID(查询records时必填)", Required: false},
	}
}
func (t *BackupListTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	listType := GetString(args, "type")
	svc := service.NewBackupService()
	if listType == "records" {
		taskID := uint(GetInt(args, "task_id"))
		records, err := svc.ListRecords(taskID)
		if err != nil {
			return ErrorErr(err)
		}
		if len(records) == 0 {
			return SuccessResult("没有备份记录")
		}
		var sb strings.Builder
		for _, r := range records {
			size := "未知"
			if r.Size > 0 {
				size = fmt.Sprintf("%.2f MB", float64(r.Size)/1024/1024)
			}
			sb.WriteString(fmt.Sprintf("ID: %d | 任务ID: %d | 文件: %s | 大小: %s | 状态: %s\n",
				r.ID, r.TaskID, r.FilePath, size, r.Status))
		}
		return SuccessResult(sb.String())
	}

	tasks, err := svc.ListTasks()
	if err != nil {
		return ErrorErr(err)
	}
	if len(tasks) == 0 {
		return SuccessResult("没有备份任务")
	}
	var sb strings.Builder
	for _, task := range tasks {
		enabled := "已启用"
		if !task.Enabled {
			enabled = "已禁用"
		}
		sb.WriteString(fmt.Sprintf("ID: %d | 名称: %s | 类型: %s | 目标: %s | 计划: %s | 保留: %d | 状态: %s\n",
			task.ID, task.Name, task.Type, task.TargetDir, task.Schedule, task.KeepCount, enabled))
	}
	return SuccessResult(sb.String())
}

// BackupOpTool 备份操作
type BackupOpTool struct{}

func NewBackupOpTool() *BackupOpTool { return &BackupOpTool{} }

func (t *BackupOpTool) Name() string { return "backup_op" }
func (t *BackupOpTool) Description() string {
	return "备份操作: run(立即执行备份)/restore(恢复备份)/delete_task/delete_record。"
}
func (t *BackupOpTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "action", Type: "string", Description: "操作: run/restore/delete_task/delete_record", Required: true},
		{Name: "id", Type: "integer", Description: "任务 ID 或记录 ID", Required: true},
	}
}
func (t *BackupOpTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	svc := service.NewBackupService()
	action := GetString(args, "action")
	id := uint(GetInt(args, "id"))
	switch action {
	case "run":
		record, err := svc.RunBackup(id)
		if err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("备份任务已执行，记录ID: %d", record.ID))
	case "restore":
		msg, err := svc.RestoreBackup(id)
		if err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(msg)
	case "delete_task":
		if err := svc.DeleteTask(id); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("备份任务 %d 已删除", id))
	case "delete_record":
		if err := svc.DeleteRecord(id); err != nil {
			return ErrorErr(err)
		}
		return SuccessResult(fmt.Sprintf("备份记录 %d 已删除", id))
	default:
		return ErrorResult("不支持的操作: %s", action)
	}
}

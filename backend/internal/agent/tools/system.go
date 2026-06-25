package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/provider"
	"github.com/minipanel/minipanel/internal/service"
	"github.com/minipanel/minipanel/internal/utils/psutil"
)

// SystemInfoTool 获取系统信息
type SystemInfoTool struct{}

func NewSystemInfoTool() *SystemInfoTool { return &SystemInfoTool{} }

func (t *SystemInfoTool) Name() string { return "get_system_info" }
func (t *SystemInfoTool) Description() string {
	return "获取服务器系统信息，包括 CPU、内存、磁盘、负载等。"
}
func (t *SystemInfoTool) Parameters() []provider.ToolParam {
	return nil
}
func (t *SystemInfoTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	svc := service.NewDashboardService()
	sys, err := svc.GetSystemInfo()
	if err != nil {
		return "", err
	}
	cpuUsage, _, err := svc.GetCPUUsage()
	if err != nil {
		cpuUsage = 0
	}
	mem, err := svc.GetMemoryInfo()
	if err != nil {
		return "", err
	}
	disks, err := svc.GetDiskInfo()
	if err != nil {
		disks = nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("主机名: %s\n", sys.Hostname))
	sb.WriteString(fmt.Sprintf("操作系统: %s %s (%s)\n", sys.OS, sys.Platform, sys.PlatformVer))
	sb.WriteString(fmt.Sprintf("架构: %s\n", sys.KernelArch))
	sb.WriteString(fmt.Sprintf("运行时间: %s\n", (time.Duration(sys.Uptime) * time.Second).String()))
	sb.WriteString(fmt.Sprintf("CPU 使用率: %.1f%%\n", cpuUsage))
	sb.WriteString(fmt.Sprintf("内存: 总计 %.1f GB, 已用 %.1f GB (%.1f%%)\n",
		float64(mem.Total)/1024/1024/1024,
		float64(mem.Used)/1024/1024/1024,
		mem.UsedPercent))
	sb.WriteString("磁盘:\n")
	for _, d := range disks {
		sb.WriteString(fmt.Sprintf("  %s: 总计 %.1f GB, 可用 %.1f GB (%.1f%% 已用)\n",
			d.Path, float64(d.Total)/1024/1024/1024,
			float64(d.Free)/1024/1024/1024, d.UsedPercent))
	}
	return sb.String(), nil
}

// ProcessListTool 获取进程列表
type ProcessListTool struct{}

func NewProcessListTool() *ProcessListTool { return &ProcessListTool{} }

func (t *ProcessListTool) Name() string { return "list_processes" }
func (t *ProcessListTool) Description() string {
	return "获取当前运行的进程列表，可按名称或 PID 过滤。"
}
func (t *ProcessListTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "filter", Type: "string", Description: "按进程名或 PID 过滤（可选）", Required: false},
	}
}
func (t *ProcessListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	svc := service.NewProcessService()
	processes, err := svc.List()
	if err != nil {
		return "", err
	}
	filter := GetString(args, "filter")
	filterLower := strings.ToLower(filter)
	var displayList []psutil.ProcessInfo
	if filter != "" {
		for _, p := range processes {
			if strings.Contains(strings.ToLower(p.Name), filterLower) ||
				fmt.Sprintf("%d", p.PID) == filter {
				displayList = append(displayList, p)
			}
		}
	} else {
		displayList = processes
	}
	if len(displayList) == 0 {
		if filter != "" {
			return fmt.Sprintf("未找到匹配 '%s' 的进程", filter), nil
		}
		return "未找到进程", nil
	}
	var sb strings.Builder
	if filter != "" {
		sb.WriteString(fmt.Sprintf("共找到 %d 个匹配 '%s' 的进程:\n", len(displayList), filter))
	} else {
		sb.WriteString(fmt.Sprintf("共 %d 个进程:\n", len(displayList)))
	}
	for _, p := range displayList {
		cmd := p.CmdLine
		if len(cmd) > 100 {
			cmd = cmd[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("PID: %d | 名称: %s | CPU: %.1f%% | 内存: %.1f%% | 命令: %s\n",
			p.PID, p.Name, p.CPUPercent, p.MemPercent, cmd))
	}
	return sb.String(), nil
}

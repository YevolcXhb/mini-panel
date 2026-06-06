package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// 危险命令黑名单
var dangerousPatterns = []string{
	"rm -rf /", "rm -rf /*", "mkfs", "dd if=/dev/zero",
	":(){:|:&};:", "shutdown", "poweroff", "halt -p",
	"> /dev/sda", "> /dev/hda", "chmod -R 777 /",
}

// ExecTool 命令执行（带安全检查）
type ExecTool struct{}

func NewExecTool() *ExecTool { return &ExecTool{} }

func (t *ExecTool) Name() string { return "execute_command" }
func (t *ExecTool) Description() string {
	return "执行 Shell 命令。支持查看系统状态、配置文件等。危险操作会被拦截。"
}
func (t *ExecTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "command", Type: "string", Description: "要执行的命令", Required: true},
		{Name: "timeout", Type: "integer", Description: "超时秒数，默认 30", Required: false},
	}
}
func (t *ExecTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	command := GetString(args, "command")
	timeout := GetInt(args, "timeout")
	if timeout <= 0 {
		timeout = 30
	}

	// 1. 危险命令拦截
	cmdLower := strings.ToLower(command)
	for _, p := range dangerousPatterns {
		if strings.Contains(cmdLower, strings.ToLower(p)) {
			return "", fmt.Errorf("危险命令被拦截: %s", p)
		}
	}

	// 2. 禁止直接操作 Docker 容器（应使用 container_op 工具）
	if strings.HasPrefix(cmdLower, "docker ") && !strings.Contains(cmdLower, "docker ps") && !strings.Contains(cmdLower, "docker images") {
		return "", fmt.Errorf("请使用 container_op 工具管理容器，不要直接执行 docker 命令")
	}

	// 3. 执行命令
	ctx2, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx2, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	result := string(output)
	if err != nil {
		if ctx2.Err() == context.DeadlineExceeded {
			return result + "\n[命令执行超时]", nil
		}
		return result + fmt.Sprintf("\n[退出码: %v]", err), nil
	}
	if result == "" {
		result = "[命令执行成功，无输出]"
	}
	return result, nil
}

// DashboardTool 获取面板概览
type DashboardTool struct{}

func NewDashboardTool() *DashboardTool { return &DashboardTool{} }

func (t *DashboardTool) Name() string { return "dashboard_overview" }
func (t *DashboardTool) Description() string {
	return "获取 MiniPanel 面板概览信息，包括容器数、网站数、数据库数等。"
}
func (t *DashboardTool) Parameters() []provider.ToolParam { return nil }
func (t *DashboardTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	// 通过 exec 获取一些基础统计
	var sb strings.Builder

	containers, _ := exec.Command("sh", "-c", "docker ps -q 2>/dev/null | wc -l").Output()
	containersAll, _ := exec.Command("sh", "-c", "docker ps -aq 2>/dev/null | wc -l").Output()

	sb.WriteString(fmt.Sprintf("运行中容器: %s", strings.TrimSpace(string(containers))))
	sb.WriteString(fmt.Sprintf(" / 总容器: %s\n", strings.TrimSpace(string(containersAll))))

	uptime, _ := exec.Command("sh", "-c", "uptime -p 2>/dev/null || uptime").Output()
	sb.WriteString(fmt.Sprintf("系统负载: %s", string(uptime)))

	return sb.String(), nil
}

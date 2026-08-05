package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/minipanel/minipanel/internal/agent/provider"
)

// 危险命令黑名单：匹配规则为命令小写后是否包含模式
var dangerousPatterns = []string{
	"rm -rf /", "rm -rf /*", "mkfs", "dd if=/dev/zero",
	":(){:|:&};:", "shutdown", "poweroff", "halt -p",
	"> /dev/sda", "> /dev/hda", "chmod -R 777 /",
}

// ExecOptions ExecTool 运行时配置（用于控制危险命令拦截和超时）
type ExecOptions struct {
	AllowDangerous bool          // true 时跳过 dangerousPatterns 拦截（仍拦截 docker）
	Timeout        time.Duration // 工具执行超时；<=0 时使用 defaultExecTimeout
}

const defaultExecTimeout = 120 * time.Second

type execOptionsCtxKey struct{}

// WithExecOptions 返回携带本次请求执行配置的 context。
// 配置随请求传递，避免并发会话之间互相覆盖。
func WithExecOptions(ctx context.Context, opts ExecOptions) context.Context {
	return context.WithValue(ctx, execOptionsCtxKey{}, opts)
}

// ExecOptionsFromContext 从 context 读取执行配置；未设置时返回默认值。
func ExecOptionsFromContext(ctx context.Context) ExecOptions {
	if opts, ok := ctx.Value(execOptionsCtxKey{}).(ExecOptions); ok {
		return opts
	}
	return ExecOptions{Timeout: defaultExecTimeout}
}

// ExecTool 命令执行（带安全检查）
type ExecTool struct{}

func NewExecTool() *ExecTool { return &ExecTool{} }

func (t *ExecTool) Name() string { return "execute_command" }
func (t *ExecTool) Description() string {
	return "执行 Shell 命令。支持查看系统状态、配置文件等。危险操作需要用户确认（可在设置中开启自动执行）。"
}
func (t *ExecTool) Parameters() []provider.ToolParam {
	return []provider.ToolParam{
		{Name: "command", Type: "string", Description: "要执行的命令", Required: true},
		{Name: "timeout", Type: "integer", Description: "超时秒数，默认 120（可在 Agent 设置中调整）", Required: false},
	}
}
func (t *ExecTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	command := GetString(args, "command")
	requestedTimeout := GetInt(args, "timeout")

	opts := ExecOptionsFromContext(ctx)
	// 优先级：参数指定 timeout > 全局 ExecTimeoutSeconds > 默认 120s
	timeout := opts.Timeout
	if requestedTimeout > 0 {
		timeout = time.Duration(requestedTimeout) * time.Second
	}
	if timeout <= 0 {
		timeout = defaultExecTimeout
	}

	cmdLower := strings.ToLower(command)
	for _, p := range dangerousPatterns {
		if strings.Contains(cmdLower, strings.ToLower(p)) {
			if opts.AllowDangerous {
				// 用户已开启危险操作自动执行，跳过拦截
				break
			}
			// 返回 confirm required 信号，让 engine 推送 confirm_required 事件给前端询问
			return ErrorResult("confirm required: 危险命令 %s 已被拦截。如需执行请在 Agent 设置中开启'允许危险操作自动执行'，或在前端确认执行。", p)
		}
	}

	if strings.HasPrefix(cmdLower, "docker ") && !strings.Contains(cmdLower, "docker ps") && !strings.Contains(cmdLower, "docker images") {
		return ErrorResult("请使用 container_op 工具管理容器，不要直接执行 docker 命令")
	}

	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx2, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	result := string(output)
	if err != nil {
		if ctx2.Err() == context.DeadlineExceeded {
			return SuccessResult(result + fmt.Sprintf("\n[命令执行超时，限制 %v]", timeout))
		}
		return SuccessResult(result + fmt.Sprintf("\n[退出码: %v]", err))
	}
	if result == "" {
		result = "[命令执行成功，无输出]"
	}
	return SuccessResult(result)
}

// DashboardTool 获取面板概览
type DashboardTool struct{}

func NewDashboardTool() *DashboardTool { return &DashboardTool{} }

func (t *DashboardTool) Name() string { return "dashboard_overview" }
func (t *DashboardTool) Description() string {
	return "获取 MiniPanel 面板概览信息，包括容器数、网站数、数据库数等。"
}
func (t *DashboardTool) Parameters() []provider.ToolParam { return nil }
func (t *DashboardTool) Execute(ctx context.Context, args map[string]interface{}) ToolExecResult {
	var sb strings.Builder

	containers, _ := exec.Command("sh", "-c", "docker ps -q 2>/dev/null | wc -l").Output()
	containersAll, _ := exec.Command("sh", "-c", "docker ps -aq 2>/dev/null | wc -l").Output()

	sb.WriteString(fmt.Sprintf("运行中容器: %s", strings.TrimSpace(string(containers))))
	sb.WriteString(fmt.Sprintf(" / 总容器: %s\n", strings.TrimSpace(string(containersAll))))

	uptime, _ := exec.Command("sh", "-c", "uptime -p 2>/dev/null || uptime").Output()
	sb.WriteString(fmt.Sprintf("系统负载: %s", string(uptime)))

	return SuccessResult(sb.String())
}

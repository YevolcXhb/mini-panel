//go:build !windows

package service

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr 在 Linux/Unix 上设置 Setsid=true，让更新进程创建新会话
// 脱离 minipanel 进程组，避免服务重启时更新进程被一起杀掉
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

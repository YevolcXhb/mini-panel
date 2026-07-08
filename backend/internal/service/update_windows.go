//go:build windows

package service

import "os/exec"

// setSysProcAttr 在 Windows 上不设置特殊参数
// Windows 不支持 setsid，更新进程会跟随父进程生命周期
// 但 Windows 通常只用于开发环境，生产环境部署在 Linux 上
func setSysProcAttr(cmd *exec.Cmd) {
	// no-op
}

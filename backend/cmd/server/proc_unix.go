//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func startProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

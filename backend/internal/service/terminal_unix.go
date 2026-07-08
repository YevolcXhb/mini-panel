//go:build !windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/gorilla/websocket"
)

func NewTerminalSession(id string, conn *websocket.Conn, shell string, role string) (*TerminalSession, error) {
	if shell == "" {
		shell = "/bin/bash"
		if _, err := os.Stat(shell); err != nil {
			shell = "/bin/sh"
		}
	}

	if ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0); err == nil {
		sess, err := newPTYSession(id, conn, shell, ptmx)
		if err != nil {
			return nil, err
		}
		sess.role = role
		return sess, nil
	}
	sess, err := newExecSession(id, conn, shell)
	if err != nil {
		return nil, err
	}
	sess.role = role
	return sess, nil
}

func newPTYSession(id string, conn *websocket.Conn, shell string, ptmx *os.File) (*TerminalSession, error) {
	defer func() {
		if ptmx != nil {
			ptmx.Close()
		}
	}()

	if err := unlockpt(ptmx); err != nil {
		return nil, fmt.Errorf("unlockpt: %w", err)
	}

	ptsName, err := ptsname(ptmx)
	if err != nil {
		return nil, fmt.Errorf("ptsname: %w", err)
	}

	pts, err := os.OpenFile(ptsName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("open pts: %w", err)
	}
	defer pts.Close()

	cmd := exec.Command(shell)
	cmd.Dir = getHomeDir()
	cmd.Env = append(os.Environ(), "TERM=xterm")
	cmd.Stdin = pts
	cmd.Stdout = pts
	cmd.Stderr = pts
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shell: %w", err)
	}

	ptmxFile := ptmx
	ptmx = nil

	sess := &TerminalSession{
		conn:   conn,
		pty:    ptmxFile,
		cmd:    cmd,
		usePTY: true,
	}

	sessionsMu.Lock()
	sessions[id] = sess
	sessionsMu.Unlock()

	go sess.readLoop()
	go sess.writeLoop()

	return sess, nil
}

func unlockpt(f *os.File) error {
	var n uint32
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&n)))
	if err != 0 {
		return err
	}
	return nil
}

func ptsname(f *os.File) (string, error) {
	var n uint32
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	if err != 0 {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

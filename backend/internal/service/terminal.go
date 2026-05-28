package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
)

type TerminalSession struct {
	conn   *websocket.Conn
	pty    *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed bool
}

var (
	sessions     = make(map[string]*TerminalSession)
	sessionsMu   sync.RWMutex
)

func NewTerminalSession(id string, conn *websocket.Conn, shell string) (*TerminalSession, error) {
	if shell == "" {
		shell = "/bin/bash"
		if _, err := os.Stat(shell); err != nil {
			shell = "/bin/sh"
		}
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm")

	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open ptmx: %w", err)
	}
	defer func() {
		if err != nil {
			ptmx.Close()
		}
	}()

	// Unlock the pseudoterminal master
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

	sess := &TerminalSession{
		conn: conn,
		pty:  ptmx,
		cmd:  cmd,
	}

	sessionsMu.Lock()
	sessions[id] = sess
	sessionsMu.Unlock()

	go sess.readLoop()
	go sess.writeLoop()

	return sess, nil
}

func (s *TerminalSession) readLoop() {
	buf := make([]byte, 1024)
	for {
		n, err := s.pty.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n[disconnect: %v]\r\n", err)))
			}
			break
		}
		s.conn.WriteMessage(websocket.BinaryMessage, buf[:n])
	}
	s.Close()
}

func (s *TerminalSession) writeLoop() {
	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			break
		}
		s.pty.Write(data)
	}
	s.Close()
}

func (s *TerminalSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if s.pty != nil {
		s.pty.Close()
	}
	s.conn.Close()
}

func GetSession(id string) (*TerminalSession, bool) {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	sess, ok := sessions[id]
	return sess, ok
}

func RemoveSession(id string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if sess, ok := sessions[id]; ok {
		sess.Close()
		delete(sessions, id)
	}
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



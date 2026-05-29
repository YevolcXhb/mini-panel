package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"sync"

	"github.com/gorilla/websocket"
)

type TerminalSession struct {
	conn   *websocket.Conn
	pty    *os.File
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed bool
	usePTY bool
}

var (
	sessions   = make(map[string]*TerminalSession)
	sessionsMu sync.RWMutex
)

func getHomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	return "/"
}

func newExecSession(id string, conn *websocket.Conn, shell string) (*TerminalSession, error) {
	cmd := exec.Command(shell)
	cmd.Dir = getHomeDir()
	cmd.Env = append(os.Environ(), "TERM=xterm")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shell: %w", err)
	}

	sess := &TerminalSession{
		conn:   conn,
		stdin:  stdin,
		stdout: stdout,
		cmd:    cmd,
		usePTY: false,
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
	var reader io.Reader
	if s.usePTY {
		reader = s.pty
	} else {
		reader = s.stdout
	}

	for {
		n, err := reader.Read(buf)
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
	var writer io.Writer
	if s.usePTY {
		writer = s.pty
	} else {
		writer = s.stdin
	}

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			break
		}
		writer.Write(data)
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
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.stdout != nil {
		s.stdout.Close()
	}
	s.conn.Close()
}

func (s *TerminalSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
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

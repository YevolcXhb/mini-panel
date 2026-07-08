//go:build windows

package service

import (
	"os"

	"github.com/gorilla/websocket"
)

func NewTerminalSession(id string, conn *websocket.Conn, shell string, role string) (*TerminalSession, error) {
	if shell == "" {
		shell = "cmd.exe"
		if _, err := os.Stat(shell); err != nil {
			shell = "powershell.exe"
		}
	}
	sess, err := newExecSession(id, conn, shell)
	if err != nil {
		return nil, err
	}
	sess.role = role
	return sess, nil
}

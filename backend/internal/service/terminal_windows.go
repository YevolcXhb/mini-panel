//go:build windows

package service

import (
	"os"

	"github.com/gorilla/websocket"
)

func NewTerminalSession(id string, conn *websocket.Conn, shell string) (*TerminalSession, error) {
	if shell == "" {
		shell = "cmd.exe"
		if _, err := os.Stat(shell); err != nil {
			shell = "powershell.exe"
		}
	}
	return newExecSession(id, conn, shell)
}

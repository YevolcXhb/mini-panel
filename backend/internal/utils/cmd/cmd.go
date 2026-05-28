package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Manager struct {
	timeout time.Duration
	ignoreError bool
}

type Option func(*Manager)

func WithTimeout(d time.Duration) Option {
	return func(m *Manager) {
		m.timeout = d
	}
}

func WithIgnoreError() Option {
	return func(m *Manager) {
		m.ignoreError = true
	}
}

func NewManager(opts ...Option) *Manager {
	m := &Manager{timeout: 30 * time.Second}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Manager) Run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil && !m.ignoreError {
		return fmt.Errorf("run %s %v: %w", name, args, err)
	}
	return nil
}

func (m *Manager) RunWithOutput(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil && !m.ignoreError {
		return string(out), fmt.Errorf("run %s %v: %w", name, args, err)
	}
	return string(out), nil
}

func Which(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func CheckIllegal(args ...string) bool {
	for _, arg := range args {
		if strings.ContainsAny(arg, "&|;$'`()\"\n\r><") {
			return true
		}
	}
	return false
}

func WriteFileWithSudo(path string, data []byte, perm os.FileMode) error {
	if err := os.WriteFile(path, data, perm); err == nil {
		return nil
	}
	cmd := exec.Command("sudo", "-n", "tee", path)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

package service

import (
	"fmt"
	"os"
	"strconv"
	"syscall"

	"github.com/minipanel/minipanel/internal/utils/psutil"
)

type ProcessService struct{}

func NewProcessService() *ProcessService {
	return &ProcessService{}
}

func (s *ProcessService) List() ([]psutil.ProcessInfo, error) {
	return psutil.GetProcesses()
}

func (s *ProcessService) Kill(pidStr string) error {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid pid: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	return proc.Signal(syscall.SIGTERM)
}

func (s *ProcessService) KillForce(pidStr string) error {
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid pid: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	return proc.Kill()
}

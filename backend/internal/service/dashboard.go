package service

import (
	"github.com/minipanel/minipanel/internal/utils/psutil"
)

type DashboardService struct{}

func NewDashboardService() *DashboardService {
	return &DashboardService{}
}

func (s *DashboardService) GetSystemInfo() (*psutil.SystemInfo, error) {
	return psutil.GetSystemInfo()
}

func (s *DashboardService) GetCPUInfo() ([]psutil.CPUInfo, error) {
	return psutil.GetCPUInfo()
}

func (s *DashboardService) GetCPUUsage() (float64, []float64, error) {
	return psutil.GetCPUUsage()
}

func (s *DashboardService) GetMemoryInfo() (*psutil.MemoryInfo, error) {
	return psutil.GetMemoryInfo()
}

func (s *DashboardService) GetDiskInfo() ([]psutil.DiskInfo, error) {
	return psutil.GetAllDiskInfo()
}

func (s *DashboardService) GetNetInfo() ([]psutil.NetInfo, error) {
	return psutil.GetNetInfo()
}

func (s *DashboardService) GetLoadAvg() (interface{}, error) {
	return psutil.GetLoadAvg()
}

func (s *DashboardService) GetMonitor() (map[string]interface{}, error) {
	cpu, _, err := psutil.GetCPUUsage()
	if err != nil {
		cpu = 0
	}
	mem, err := psutil.GetMemoryInfo()
	if err != nil {
		mem = &psutil.MemoryInfo{}
	}
	disks, err := psutil.GetAllDiskInfo()
	if err != nil {
		disks = []psutil.DiskInfo{}
	}
	net, err := psutil.GetNetInfo()
	if err != nil {
		net = []psutil.NetInfo{}
	}
	load, _ := psutil.GetLoadAvg()

	return map[string]interface{}{
		"cpu_usage":  cpu,
		"memory":     mem,
		"disks":      disks,
		"network":    net,
		"load":       load,
	}, nil
}

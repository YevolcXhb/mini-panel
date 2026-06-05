package service

import (
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/utils/psutil"
)

type MonitorService struct{}

func NewMonitorService() *MonitorService {
	return &MonitorService{}
}

func (s *MonitorService) Record() error {
	cpuUsage, _, err := psutil.GetCPUUsage()
	if err != nil {
		cpuUsage = 0
	}
	mem, err := psutil.GetMemoryInfo()
	if err != nil {
		mem = &psutil.MemoryInfo{}
	}
	disks, err := psutil.GetAllDiskInfo()
	if err != nil {
		disks = []psutil.DiskInfo{}
	}
	var diskUsed, diskTotal uint64
	for _, d := range disks {
		diskUsed += d.Used
		diskTotal += d.Total
	}
	net, err := psutil.GetNetInfo()
	if err != nil {
		net = []psutil.NetInfo{}
	}
	var netSent, netRecv uint64
	for _, n := range net {
		netSent += n.BytesSent
		netRecv += n.BytesRecv
	}
	load, _ := psutil.GetLoadAvg()

	record := &model.MonitorHistory{
		CPUUsage:   cpuUsage,
		MemUsed:    mem.Used,
		MemTotal:   mem.Total,
		DiskUsed:   diskUsed,
		DiskTotal:  diskTotal,
		NetSent:    netSent,
		NetRecv:    netRecv,
		Load1:      load.Load1,
		Load5:      load.Load5,
		Load15:     load.Load15,
		RecordedAt: time.Now(),
	}
	return global.DB.Create(record).Error
}

func (s *MonitorService) List(limit int) ([]model.MonitorHistory, error) {
	if limit <= 0 {
		limit = 1440
	}
	var items []model.MonitorHistory
	err := global.DB.Order("recorded_at DESC").Limit(limit).Find(&items).Error
	if err != nil {
		return nil, err
	}
	// reverse to ascending order for charts
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

func (s *MonitorService) Cleanup(days int) error {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	return global.DB.Where("recorded_at < ?", cutoff).Delete(&model.MonitorHistory{}).Error
}

func StartMonitorCollector() {
	svc := NewMonitorService()
	ticker := time.NewTicker(time.Minute)
	go func() {
		// record immediately on start
		_ = svc.Record()
		_ = svc.Cleanup(7)
		for range ticker.C {
			_ = svc.Record()
			_ = svc.Cleanup(7)
		}
	}()
}

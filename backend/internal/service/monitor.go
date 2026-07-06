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

	// RealtimeMetrics 实时指标
	type RealtimeMetrics struct {
		CPUPercent   float64 `json:"cpu_percent"`
		MemUsed      uint64  `json:"mem_used"`
		MemTotal     uint64  `json:"mem_total"`
		MemPercent   float64 `json:"mem_percent"`
		DiskUsed     uint64  `json:"disk_used"`
		DiskTotal    uint64  `json:"disk_total"`
		DiskPercent  float64 `json:"disk_percent"`
		NetSent      uint64  `json:"net_sent"`
		NetRecv      uint64  `json:"net_recv"`
		NetSentSpeed uint64  `json:"net_sent_speed"`
		NetRecvSpeed uint64  `json:"net_recv_speed"`
		Load1        float64 `json:"load1"`
		Load5        float64 `json:"load5"`
		Load15       float64 `json:"load15"`
	}

	// GetRealtime 获取实时系统指标
	func (s *MonitorService) GetRealtime() (*RealtimeMetrics, error) {
		cpuUsage, _, err := psutil.GetCPUUsage()
		if err != nil {
			cpuUsage = 0
		}
		mem, err := psutil.GetMemoryInfo()
		if err != nil {
			mem = &psutil.MemoryInfo{}
		}
		var memPercent float64
		if mem.Total > 0 {
			memPercent = float64(mem.Used) / float64(mem.Total) * 100
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
		var diskPercent float64
		if diskTotal > 0 {
			diskPercent = float64(diskUsed) / float64(diskTotal) * 100
		}

		// 第一次采样网络
		net1, _ := psutil.GetNetInfo()
		net1Sent, net1Recv := sumNet(net1)
		time.Sleep(1 * time.Second)
		// 第二次采样
		net2, _ := psutil.GetNetInfo()
		net2Sent, net2Recv := sumNet(net2)

		load, _ := psutil.GetLoadAvg()

		return &RealtimeMetrics{
			CPUPercent:   cpuUsage,
			MemUsed:      mem.Used,
			MemTotal:     mem.Total,
			MemPercent:   memPercent,
			DiskUsed:     diskUsed,
			DiskTotal:    diskTotal,
			DiskPercent:  diskPercent,
			NetSent:      net2Sent,
			NetRecv:      net2Recv,
			NetSentSpeed: net2Sent - net1Sent,
			NetRecvSpeed: net2Recv - net1Recv,
			Load1:        load.Load1,
			Load5:        load.Load5,
			Load15:       load.Load15,
		}, nil
	}

	func sumNet(netInfo []psutil.NetInfo) (sent, recv uint64) {
		for _, n := range netInfo {
			sent += n.BytesSent
			recv += n.BytesRecv
		}
		return
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

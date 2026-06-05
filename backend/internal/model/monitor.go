package model

import "time"

type MonitorHistory struct {
	BaseModel
	CPUUsage    float64 `json:"cpu_usage"`
	MemUsed     uint64  `json:"mem_used"`
	MemTotal    uint64  `json:"mem_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskTotal   uint64  `json:"disk_total"`
	NetSent     uint64  `json:"net_sent"`
	NetRecv     uint64  `json:"net_recv"`
	Load1       float64 `json:"load1"`
	Load5       float64 `json:"load5"`
	Load15      float64 `json:"load15"`
	RecordedAt  time.Time `json:"recorded_at"`
}

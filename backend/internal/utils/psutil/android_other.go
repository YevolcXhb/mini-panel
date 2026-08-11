//go:build !linux

package psutil

import "github.com/shirou/gopsutil/v4/disk"

func getDiskPartitions() ([]disk.PartitionStat, string, error) {
	partitions, err := disk.Partitions(false)
	return partitions, "", err
}

func androidHostRoot() string {
	return ""
}

//go:build !linux

package psutil

import (
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

// platformPartitionKey 返回非 Linux 平台（如 Windows）的设备去重标识。
func platformPartitionKey(p disk.PartitionStat) string {
	device := strings.ToLower(strings.TrimSpace(p.Device))
	return strings.TrimRight(device, `/\`)
}

//go:build linux

package psutil

import (
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

// platformPartitionKey 返回 Linux 下用于磁盘去重的稳定标识。
// /dev/root 等设备名别名在解析宿主机 mountinfo 时已通过
// /sys/dev/block/<major:minor> 转换为真实设备名，这里再做一次符号链接兜底。
func platformPartitionKey(p disk.PartitionStat) string {
	device := strings.TrimSpace(p.Device)
	if device == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(device); err == nil && resolved != "" {
		device = resolved
	}
	return strings.ToLower(strings.TrimRight(device, "/"))
}

func unescapeMountinfoPath(s string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(s)
}

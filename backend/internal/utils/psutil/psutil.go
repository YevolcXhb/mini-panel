package psutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type SystemInfo struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Platform      string `json:"platform"`
	PlatformVer   string `json:"platform_version"`
	KernelArch    string `json:"kernel_arch"`
	KernelVersion string `json:"kernel_version"`
	Uptime        uint64 `json:"uptime"`
	BootTime      uint64 `json:"boot_time"`
	Procs         uint64 `json:"procs"`
}

type CPUInfo struct {
	ModelName string  `json:"model_name"`
	Cores     int32   `json:"cores"`
	Mhz       float64 `json:"mhz"`
}

type MemoryInfo struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"used_percent"`
}

type DiskInfo struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
	FSType      string  `json:"fs_type"`
}

type NetInfo struct {
	Name        string `json:"name"`
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
}

type ProcessInfo struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float32 `json:"mem_percent"`
	Status     string  `json:"status"`
	CmdLine    string  `json:"cmdline"`
}

func GetSystemInfo() (*SystemInfo, error) {
	info, err := host.Info()
	if err != nil {
		return nil, err
	}
	return &SystemInfo{
		Hostname:      info.Hostname,
		OS:            info.OS,
		Platform:      info.Platform,
		PlatformVer:   info.PlatformVersion,
		KernelArch:    info.KernelArch,
		KernelVersion: info.KernelVersion,
		Uptime:        info.Uptime,
		BootTime:      info.BootTime,
		Procs:         info.Procs,
	}, nil
}

func GetCPUInfo() ([]CPUInfo, error) {
	infos, err := cpu.Info()
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		// Fallback for ARM/Android where cpu.Info() may return empty
		return []CPUInfo{{ModelName: "ARM", Cores: 1}}, nil
	}
	var result []CPUInfo
	for _, info := range infos {
		result = append(result, CPUInfo{
			ModelName: info.ModelName,
			Cores:     info.Cores,
			Mhz:       info.Mhz,
		})
	}
	return result, nil
}

func GetCPUUsage() (float64, []float64, error) {
	percent, err := cpu.Percent(500*time.Millisecond, true)
	if err != nil {
		return 0, nil, err
	}
	var total float64
	for _, p := range percent {
		total += p
	}
	if len(percent) > 0 {
		total /= float64(len(percent))
	}
	return total, percent, nil
}

func GetMemoryInfo() (*MemoryInfo, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	return &MemoryInfo{
		Total:       v.Total,
		Used:        v.Used,
		Free:        v.Free,
		Available:   v.Available,
		UsedPercent: v.UsedPercent,
	}, nil
}

func GetDiskInfo(path string) (*DiskInfo, error) {
	if path == "" {
		path = "/"
	}
	usage, err := disk.Usage(path)
	if err != nil {
		return nil, err
	}
	return &DiskInfo{
		Path:        usage.Path,
		Total:       usage.Total,
		Used:        usage.Used,
		Free:        usage.Free,
		UsedPercent: usage.UsedPercent,
		FSType:      usage.Fstype,
	}, nil
}

func GetAllDiskInfo() ([]DiskInfo, error) {
	partitions, hostRoot, err := getDiskPartitions()
	if err != nil {
		return nil, err
	}
	var result []DiskInfo
	for _, p := range selectPhysicalPartitions(partitions) {
		usagePath := p.Mountpoint
		if hostRoot != "" {
			// Android chroot：通过宿主机逃逸路径执行 statfs，
			// 否则 / 与 /data 都会解析到 chroot 所在的同一块用户分区
			usagePath = filepath.Join(hostRoot, p.Mountpoint)
		}
		usage, err := disk.Usage(usagePath)
		if err != nil {
			continue
		}
		result = append(result, DiskInfo{
			Path:        p.Mountpoint,
			Total:       usage.Total,
			Used:        usage.Used,
			Free:        usage.Free,
			UsedPercent: usage.UsedPercent,
			FSType:      usage.Fstype,
		})
	}
	return result, nil
}

// selectPhysicalPartitions 过滤虚拟文件系统，并按底层设备去重。
// 同一设备可能以多个挂载点出现（如 / 与 /data 是同一块 f2fs 的绑定挂载），
// 若逐个统计会导致磁盘容量和用量被重复累加。
func selectPhysicalPartitions(partitions []disk.PartitionStat) []disk.PartitionStat {
	var result []disk.PartitionStat
	seen := make(map[string]int) // normalized device -> index in result
	for _, p := range partitions {
		if isVirtualFSType(p.Fstype) || isAndroidMirrorMount(p) {
			continue
		}
		key := normalizeDeviceKey(p)
		if idx, ok := seen[key]; ok {
			// 同一设备保留层级更浅的挂载点（如 / 优先于 /data）
			if preferMountpoint(p.Mountpoint, result[idx].Mountpoint) {
				result[idx] = p
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, p)
	}
	return result
}

// isAndroidMirrorMount 判断是否为 Android 内部存储的 FUSE/sdcardfs 镜像挂载
// （/storage/emulated、/mnt/runtime 等）。它们的内容来自 /data/media，
// 与 /data 是同一块物理存储，重复计入会让磁盘总量和用量虚高。
func isAndroidMirrorMount(p disk.PartitionStat) bool {
	fsType := strings.ToLower(strings.TrimSpace(p.Fstype))
	if fsType != "fuse" && fsType != "sdcardfs" {
		return false
	}
	mp := p.Mountpoint
	return strings.HasPrefix(mp, "/storage/emulated") ||
		strings.HasPrefix(mp, "/mnt/runtime/") ||
		strings.HasPrefix(mp, "/mnt/user/")
}

func isVirtualFSType(fsType string) bool {
	switch strings.ToLower(strings.TrimSpace(fsType)) {
	case "tmpfs", "devtmpfs", "devfs", "proc", "sysfs", "cgroup", "cgroup2",
		"devpts", "ramfs", "shm", "mqueue", "debugfs", "tracefs", "securityfs",
		"pstore", "hugetlbfs", "autofs", "binfmt_misc", "configfs", "fusectl",
		"nsfs", "overlay", "squashfs", "sockfs", "pipefs", "bpf", "rpc_pipefs":
		return true
	}
	return false
}

// normalizeDeviceKey 生成设备去重键；设备名为空时退回“文件系统类型+挂载点”，
// 避免把所有未知设备合并成一个。
func normalizeDeviceKey(p disk.PartitionStat) string {
	if key := platformPartitionKey(p); key != "" {
		return key
	}
	return strings.ToLower(strings.TrimSpace(p.Fstype)) + ":" + strings.TrimRight(p.Mountpoint, `/\`)
}

// preferMountpoint 返回 true 表示 a 比 b 更适合作为代表挂载点。
// 优先选择路径更短的（/ 优于 /data，C:\ 优于 C:\subdir）。
func preferMountpoint(a, b string) bool {
	a = strings.TrimRight(a, `/\`)
	b = strings.TrimRight(b, `/\`)
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func GetNetInfo() ([]NetInfo, error) {
	ioCounters, err := net.IOCounters(true)
	if err != nil {
		return nil, err
	}
	var result []NetInfo
	for _, io := range ioCounters {
		result = append(result, NetInfo{
			Name:        io.Name,
			BytesSent:   io.BytesSent,
			BytesRecv:   io.BytesRecv,
			PacketsSent: io.PacketsSent,
			PacketsRecv: io.PacketsRecv,
		})
	}
	return result, nil
}

func GetLoadAvg() (*load.AvgStat, error) {
	stat, err := load.Avg()
	if err == nil && stat != nil {
		return stat, nil
	}
	// Fallback: read /proc/loadavg directly
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return &load.AvgStat{Load1: 0, Load5: 0, Load15: 0}, nil
	}
	var l1, l5, l15 float64
	fmt.Sscanf(string(data), "%f %f %f", &l1, &l5, &l15)
	return &load.AvgStat{Load1: l1, Load5: l5, Load15: l15}, nil
}

func GetProcesses() ([]ProcessInfo, error) {
	defer func() {
		if r := recover(); r != nil {
			// gopsutil may panic on some Android systems
		}
	}()

	procs, err := process.Processes()
	if err != nil {
		return getProcessesFromProc()
	}
	var result []ProcessInfo
	for _, p := range procs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					// skip problematic processes
				}
			}()
			name, _ := p.Name()
			cpu, _ := p.CPUPercent()
			mem, _ := p.MemoryPercent()
			status, _ := p.Status()
			cmdline, _ := p.Cmdline()
			result = append(result, ProcessInfo{
				PID:        p.Pid,
				Name:       name,
				CPUPercent: cpu,
				MemPercent: mem,
				Status:     strings.Join(status, ","),
				CmdLine:    cmdline,
			})
		}()
	}
	if len(result) == 0 {
		return getProcessesFromProc()
	}
	return result, nil
}

func getProcessesFromProc() ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var result []ProcessInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		comm, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		name := strings.TrimSpace(string(comm))
		if name == "" {
			cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
			name = strings.ReplaceAll(string(cmdline), "\x00", " ")
		}
		result = append(result, ProcessInfo{
			PID:  int32(pid),
			Name: name,
		})
	}
	return result, nil
}

func GetDistro() string {
	files := []string{"/etc/os-release", "/usr/lib/os-release"}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				v := strings.TrimPrefix(line, "PRETTY_NAME=")
				return strings.Trim(v, `"`)
			}
		}
	}
	info, _ := host.Info()
	return fmt.Sprintf("%s %s", info.Platform, info.PlatformVersion)
}

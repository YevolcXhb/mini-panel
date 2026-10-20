package psutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
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
		if usage.Total == 0 {
			// 容量为 0 的伪分区（selinuxfs、binderfs、functionfs 等）对用户没有意义
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
		"nsfs", "overlay", "squashfs", "sockfs", "pipefs", "bpf", "rpc_pipefs",
		"selinuxfs", "binder", "binderfs", "functionfs", "usb-ffs", "efivarfs":
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
	if err != nil || len(ioCounters) == 0 {
		// 容器内独立网络命名空间时本进程的 /proc/net/dev 可能不存在或为空，
		// 退回读取 Android 宿主机逃逸根下的 /proc/net/dev。
		if fallback, ferr := readHostNetDev(); ferr == nil && len(fallback) > 0 {
			return fallback, nil
		}
		if err != nil {
			return nil, err
		}
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

// readHostNetDev 直接从 /proc/net/dev 解析网络计数。
// gopsutil 的 net.IOCounters 读取的是本进程所在网络命名空间的 /proc/net/dev；
// 面板运行在独立网络命名空间（容器）中时该视图可能是空的，此处改读 Android
// 宿主机逃逸根下的 /proc/net/dev。
//
// /proc/net/dev 格式（前两行为表头）：
//
//	Inter-|   Receive                                                |  Transmit
//	 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets ...
//	  wlan0: 1983640 12345 0 0 0 0 0 0 3902770 23456 0 0 0 0 0 0
func readHostNetDev() ([]NetInfo, error) {
	candidates := []string{"/proc/net/dev"}
	if root := androidHostRoot(); root != "" {
		candidates = append([]string{filepath.Join(root, "proc/net/dev")}, candidates...)
	}
	var lastErr error
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		result := parseNetDev(data)
		if len(result) > 0 {
			return result, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("proc/net/dev unavailable")
	}
	return nil, lastErr
}

func parseNetDev(data []byte) []NetInfo {
	var result []NetInfo
	for _, line := range strings.Split(string(data), "\n") {
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue // 表头行
		}
		name := strings.TrimSpace(line[:idx])
		if name == "" {
			continue
		}
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 16 {
			continue
		}
		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		rxPackets, _ := strconv.ParseUint(fields[1], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		txPackets, _ := strconv.ParseUint(fields[9], 10, 64)
		result = append(result, NetInfo{
			Name:        name,
			BytesSent:   txBytes,
			BytesRecv:   rxBytes,
			PacketsSent: txPackets,
			PacketsRecv: rxPackets,
		})
	}
	return result
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

// ─── 进程列表（自算 CPU 差值 + 快照缓存）───
//
// 为什么不用 gopsutil 的 p.CPUPercent()：
//   gopsutil 的 CPUPercent 是"相对上次调用"的差值计算，每次都要重新读
//   /proc/<pid>/stat、/proc/uptime、/proc/stat 并持锁采样。对本机 636 个
//   进程逐个调用时，单次 /processes 请求实测触发 14810 次文件读取
//   （整轮 /proc 扫描只要 636 次），读盘速率 4MB/s，直接把 2.5 个 CPU 核
//   空转烧满。
//
// 现在的做法：每轮只把 /proc/<pid>/stat 读一遍，用 总jiffies/系统总jiffies
// 自行求百分比；两次调用之间的差值由内存快照提供，不再额外读任何全局文件。
// 并且整个结果缓存 procCacheTTL，多个前端实例并发轮询时只算一次。

// procSnapshot 保存上一轮采样，用于计算 CPU 占用差值。
type procSnapshot struct {
	totalJiffies uint64
	procJiffies  map[int32]uint64
	at           time.Time
}

var (
	procCacheMu   sync.RWMutex
	procCacheList []ProcessInfo
	procCacheAt   time.Time
	procPrevSnap  *procSnapshot
)

const (
	// procCacheTTL 进程列表缓存时长。前端有多处轮询，10 秒足够新，
	// 又能把并发请求合并成一次全量扫描。
	procCacheTTL = 10 * time.Second
	// procMinSampleGap 两次采样最小间隔，太近的采样差值噪声大。
	procMinSampleGap = 500 * time.Millisecond
)

// clockTicks 每秒的 jiffies 数。Linux 用户态固定为 100，Android 同样。
const clockTicks = 100

// totalCPUTicks 读取 /proc/stat 第一行，汇总所有 cpuN 的 jiffies 总和。
func totalCPUTicks() uint64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	line := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		line = data[:i]
	}
	fields := strings.Fields(string(line))
	var sum uint64
	for i, f := range fields {
		if i == 0 {
			continue // "cpu"
		}
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			continue
		}
		sum += v
	}
	return sum
}

// readProcStat 从 /proc/<pid>/stat 解析 comm、状态、utime+stime、rss。
//
// comm 字段可能包含空格和括号，所以必须从最后一个 ')' 之后开始切分，
// 不能简单地按空白分隔。
func readProcStat(pid int32) (name, status string, jiffies uint64, rssPages uint64, ok bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", "", 0, 0, false
	}
	s := string(data)
	lp := strings.LastIndexByte(s, ')')
	open := strings.IndexByte(s, '(')
	if open < 0 || lp < 0 || lp <= open {
		return "", "", 0, 0, false
	}
	name = s[open+1 : lp]
	// ')' 之后的字段从第 3 个字段（state）开始
	rest := strings.Fields(s[lp+1:])
	if len(rest) < 22 {
		return "", "", 0, 0, false
	}
	status = rest[0]
	// utime 是第 14 个字段，stime 第 15；rest[0] 对应第 3 个字段
	utime, _ := strconv.ParseUint(rest[11], 10, 64)
	stime, _ := strconv.ParseUint(rest[12], 10, 64)
	rss, _ := strconv.ParseUint(rest[21], 10, 64)
	return name, status, utime + stime, rss, true
}

// GetProcesses 返回进程列表。
//
// 结果按 procCacheTTL 缓存；CPU 百分比为"距上一轮采样"的平均占用，
// 首轮采样没有基线，CPU 一律返回 0（而不是假的 100%）。
func GetProcesses() ([]ProcessInfo, error) {
	procCacheMu.RLock()
	if procCacheList != nil && time.Since(procCacheAt) < procCacheTTL {
		cached := make([]ProcessInfo, len(procCacheList))
		copy(cached, procCacheList)
		procCacheMu.RUnlock()
		return cached, nil
	}
	procCacheMu.RUnlock()

	procCacheMu.Lock()
	defer procCacheMu.Unlock()
	// 双检：可能在等锁期间别的 goroutine 已经刷新好了
	if procCacheList != nil && time.Since(procCacheAt) < procCacheTTL {
		cached := make([]ProcessInfo, len(procCacheList))
		copy(cached, procCacheList)
		return cached, nil
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		if procCacheList != nil {
			cached := make([]ProcessInfo, len(procCacheList))
			copy(cached, procCacheList)
			return cached, nil
		}
		return nil, err
	}

	totalNow := totalCPUTicks()
	now := time.Now()

	// 上一次快照距今太近时不做差，直接沿用旧 CPU 值，避免噪声
	canDiff := procPrevSnap != nil &&
		totalNow > procPrevSnap.totalJiffies &&
		now.Sub(procPrevSnap.at) >= procMinSampleGap

	memTotal := totalMemoryBytes()

	curJiffies := make(map[int32]uint64, len(entries))
	result := make([]ProcessInfo, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		pid64, perr := strconv.Atoi(name)
		if perr != nil {
			continue
		}
		pid := int32(pid64)
		pname, pstatus, jiffies, rssPages, ok := readProcStat(pid)
		if !ok {
			continue // 进程可能在扫描期间退出
		}
		curJiffies[pid] = jiffies

		var cpuPct float64
		if canDiff {
			if prev, seen := procPrevSnap.procJiffies[pid]; seen && jiffies >= prev {
				deltaProc := jiffies - prev
				deltaTotal := totalNow - procPrevSnap.totalJiffies
				if deltaTotal > 0 {
					// 单核为 100%，多核可能超过 100%（与 top 口径一致）
					cpuPct = float64(deltaProc) / float64(deltaTotal) * 100 *
						float64(runtime.NumCPU())
				}
			}
		}

		var memPct float32
		if memTotal > 0 && rssPages > 0 {
			// rss 单位是页，页大小取系统页
			rssBytes := float64(rssPages) * float64(os.Getpagesize())
			memPct = float32(rssBytes / float64(memTotal) * 100)
		}

		cmdline := ""
		if raw, e := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); e == nil {
			cmdline = strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", " "))
			if len(cmdline) > 512 {
				cmdline = cmdline[:512]
			}
		}

		result = append(result, ProcessInfo{
			PID:        pid,
			Name:       pname,
			CPUPercent: cpuPct,
			MemPercent: memPct,
			Status:     pstatus,
			CmdLine:    cmdline,
		})
	}

	procPrevSnap = &procSnapshot{
		totalJiffies: totalNow,
		procJiffies:  curJiffies,
		at:           now,
	}
	procCacheList = result
	procCacheAt = now

	out := make([]ProcessInfo, len(result))
	copy(out, result)
	return out, nil
}

// totalMemoryBytes 读取 /proc/meminfo 的 MemTotal（kB → 字节）。
func totalMemoryBytes() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
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

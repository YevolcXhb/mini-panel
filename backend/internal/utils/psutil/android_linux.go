//go:build linux

package psutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

var (
	hostRootMu    sync.Mutex
	hostRootCache string
	hostRootAt    time.Time
)

const hostRootCacheTTL = 30 * time.Second

// getDiskPartitions 在 Android chroot 环境中返回宿主机视角的挂载点列表，
// 普通 Linux/Windows 环境直接使用 gopsutil 枚举。
func getDiskPartitions() ([]disk.PartitionStat, string, error) {
	hostRoot := androidHostRoot()
	if hostRoot == "" {
		partitions, err := disk.Partitions(false)
		return partitions, "", err
	}
	partitions, err := readHostPartitions(hostRoot)
	if err == nil && len(partitions) > 0 {
		return partitions, hostRoot, nil
	}
	// 逃逸读取失败时回退到 chroot 内枚举，避免磁盘详情完全消失
	partitions, err = disk.Partitions(false)
	return partitions, "", err
}

// androidHostRoot 返回可访问的 Android 宿主机根目录（带短时缓存）。
// magiskd 进程优先，其次为 init（/proc/1/root）。
func androidHostRoot() string {
	hostRootMu.Lock()
	defer hostRootMu.Unlock()
	if hostRootCache != "" && time.Since(hostRootAt) < hostRootCacheTTL {
		return hostRootCache
	}
	root := detectAndroidHostRoot()
	hostRootCache = root
	hostRootAt = time.Now()
	return root
}

func detectAndroidHostRoot() string {
	if !looksLikeAndroidChroot() {
		return ""
	}
	if pid := findProcessPID("magiskd"); pid > 0 {
		root := fmt.Sprintf("/proc/%d/root", pid)
		if isAndroidRoot(root) {
			return root
		}
	}
	if isAndroidRoot("/proc/1/root") {
		return "/proc/1/root"
	}
	return ""
}

func looksLikeAndroidChroot() bool {
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/version"); err == nil {
		if strings.Contains(strings.ToLower(string(data)), "android") {
			return true
		}
	}
	// 共享内核的 Android 环境中，/proc/1/root 指向 Android init 的根目录
	if _, err := os.Stat("/proc/1/root/system"); err == nil {
		return true
	}
	return false
}

func isAndroidRoot(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "system")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(root, "system/build.prop")); err == nil {
		return true
	}
	return false
}

func findProcessPID(name string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		pidStr := e.Name()
		if pidStr == "" || pidStr[0] < '0' || pidStr[0] > '9' {
			continue
		}
		if data, err := os.ReadFile("/proc/" + pidStr + "/comm"); err == nil {
			if strings.TrimSpace(string(data)) == name {
				pid, _ := strconv.Atoi(pidStr)
				if pid > 0 {
					return pid
				}
			}
		}
		if data, err := os.ReadFile("/proc/" + pidStr + "/cmdline"); err == nil {
			if strings.Contains(strings.ReplaceAll(string(data), "\x00", " "), name) {
				pid, _ := strconv.Atoi(pidStr)
				if pid > 0 {
					return pid
				}
			}
		}
	}
	return 0
}

// readHostPartitions 读取宿主机挂载表。
// 优先读取跳板进程（magiskd/init）自身的 /proc/<pid>/mountinfo，
// 保证拿到的是宿主机完整挂载命名空间，而不是面板进程受限后的视角；
// 失败时退回通过逃逸路径读取 /proc/self/mountinfo，最后退回 /proc/self/mounts。
func readHostPartitions(root string) ([]disk.PartitionStat, error) {
	if pid := pidFromHostRoot(root); pid > 0 {
		if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/mountinfo", pid)); err == nil {
			if partitions, perr := parseMountinfo(data, root); perr == nil && len(partitions) > 0 {
				return partitions, nil
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "proc/self/mountinfo"))
	if err == nil {
		return parseMountinfo(data, root)
	}
	data, err = os.ReadFile(filepath.Join(root, "proc/self/mounts"))
	if err != nil {
		return nil, err
	}
	return parseMounts(data)
}

// pidFromHostRoot 从 /proc/<pid>/root 形式的逃逸根路径中提取跳板进程 PID。
func pidFromHostRoot(root string) int {
	parts := strings.Split(strings.TrimRight(root, "/"), "/")
	if len(parts) < 3 || parts[len(parts)-1] != "root" {
		return 0
	}
	pid, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0
	}
	return pid
}

func parseMountinfo(data []byte, root string) ([]disk.PartitionStat, error) {
	var ret []disk.PartitionStat
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, " - ")
		if len(parts) != 2 {
			continue
		}
		left := strings.Fields(parts[0])
		right := strings.Fields(parts[1])
		if len(left) < 6 || len(right) < 2 {
			continue
		}
		opts := strings.Split(left[5], ",")
		if rootDir := left[3]; rootDir != "" && rootDir != "/" {
			opts = append(opts, "bind")
		}
		p := disk.PartitionStat{
			Device:     right[1],
			Mountpoint: unescapeMountinfoPath(left[4]),
			Fstype:     right[0],
			Opts:       opts,
		}
		// /dev/root 不是真实设备名，通过宿主机 sysfs 的主次号解析出真实块设备
		if p.Device == "/dev/root" && root != "" && len(left) > 2 {
			if devpath, err := os.Readlink(filepath.Join(root, "sys/dev/block/", left[2])); err == nil && devpath != "" {
				p.Device = "/dev/" + filepath.Base(devpath)
			}
		}
		ret = append(ret, p)
	}
	return ret, nil
}

func parseMounts(data []byte) ([]disk.PartitionStat, error) {
	var ret []disk.PartitionStat
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ret = append(ret, disk.PartitionStat{
			Device:     fields[0],
			Mountpoint: unescapeMountinfoPath(fields[1]),
			Fstype:     fields[2],
			Opts:       strings.Fields(fields[3]),
		})
	}
	return ret, nil
}

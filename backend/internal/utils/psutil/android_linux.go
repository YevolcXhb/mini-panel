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
// 不依赖任何固定 PID：候选路径必须通过 isAndroidRoot 校验（存在 /system/bin/init
// 或 /system/build.prop）才会被采用。
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
	// magiskd 是文档推荐的逃逸入口，先试它。
	if pid := findProcessPID("magiskd"); pid > 0 {
		root := fmt.Sprintf("/proc/%d/root", pid)
		if isAndroidRoot(root) {
			return root
		}
	}
	// /proc/1/root 只在它确实指向 Android 根时才采用：容器（dockroot 等）里
	// pid 1 是容器自己的 systemd，其 root 不含 /system/bin/init，会被校验拒绝。
	if isAndroidRoot("/proc/1/root") {
		return "/proc/1/root"
	}
	if !looksLikeAndroidChroot() {
		return ""
	}
	// 兜底：扫描全部 /proc/*/root，适用于 magiskd 不可见、pid 1 被容器占用的情形。
	if roots := scanAndroidRoots(); len(roots) > 0 {
		return roots[0]
	}
	return ""
}

// scanAndroidRoots 扫描 /proc/*/root，返回所有指向 Android 根目录的逃逸路径。
// 容器或独立 PID 命名空间下 magiskd/init 可能不可见或指向错误对象，
// 因此这里按内容特征（/system/bin/init、/system/build.prop）而不是 PID 来识别。
func scanAndroidRoots() []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var roots []string
	for _, e := range entries {
		name := e.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		root := "/proc/" + name + "/root"
		if isAndroidRoot(root) {
			roots = append(roots, root)
		}
	}
	return roots
}

func looksLikeAndroidChroot() bool {
	if _, err := os.Stat("/system/build.prop"); err == nil {
		return true
	}
	// chroot 容器（如 Ubuntu chroot 模块）会注入这些变量，即使容器内看不到 /system
	if os.Getenv("ANDROID_ROOT") != "" || os.Getenv("TMOE_CHROOT") != "" {
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

// isAndroidRoot 判断 root 是否为 Android 宿主机根目录。
// 用 /system/bin/init 或 /system/build.prop 作特征：容器 rootfs（Ubuntu 等）
// 即使有 /system 目录也不会有 init 与 build.prop。
func isAndroidRoot(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "system/build.prop")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(root, "system/bin/init")); err == nil {
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
// 依次尝试多个来源（逃逸根下的 /proc/<pid>/mountinfo、<root>/proc/self/mountinfo、
// <root>/proc/mounts，以及本进程可见的 /proc/1/mountinfo、/proc/self/mountinfo），
// 取条目最多的那一份。任何单一来源都可能是残缺的：例如面板进程处于独立挂载命名
// 空间时 /proc/self/mountinfo 只有若干虚拟挂载项，直接采用会导致磁盘详情为空。
func readHostPartitions(root string) ([]disk.PartitionStat, error) {
	type source struct {
		path   string
		parser func([]byte, string) ([]disk.PartitionStat, error)
	}
	var sources []source
	if root != "" {
		if pid := pidFromHostRoot(root); pid > 0 {
			sources = append(sources,
				source{fmt.Sprintf("/proc/%d/mountinfo", pid), parseMountinfo},
				source{fmt.Sprintf("/proc/%d/mounts", pid), parseMountsStrict},
			)
		}
		sources = append(sources,
			source{filepath.Join(root, "proc/self/mountinfo"), parseMountinfo},
			source{filepath.Join(root, "proc/mounts"), parseMountsStrict},
		)
	}
	sources = append(sources,
		source{"/proc/1/mountinfo", parseMountinfo},
		source{"/proc/self/mountinfo", parseMountinfo},
	)

	var best []disk.PartitionStat
	var lastErr error
	for _, src := range sources {
		data, err := os.ReadFile(src.path)
		if err != nil {
			lastErr = err
			continue
		}
		partitions, err := src.parser(data, root)
		if err != nil {
			lastErr = err
			continue
		}
		if len(partitions) > len(best) {
			best = partitions
		}
	}
	if len(best) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("no readable mount table for root %q", root)
		}
		return nil, lastErr
	}
	return best, nil
}

// parseMountsStrict 解析 /proc/mounts 格式；与 parseMounts 相比会丢弃 opt 字段落
// 在 fstype 前缀上的注释行（/proc/mounts 无注释，此处仅为签名统一）。
func parseMountsStrict(data []byte, _ string) ([]disk.PartitionStat, error) {
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

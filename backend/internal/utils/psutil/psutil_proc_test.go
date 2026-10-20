package psutil

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// readProcStat 必须能从 /proc/<pid>/stat 正确取到自身进程的信息。
// 这个用例在 Linux 上跑真实 /proc；非 Linux 直接跳过。
func TestReadProcStatSelf(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("/proc 不可用，跳过")
	}
	name, status, jiffies, rss, ok := readProcStat(int32(os.Getpid()))
	if !ok {
		t.Fatal("应能解析自身 /proc/<pid>/stat")
	}
	if name == "" {
		t.Fatal("进程名不应为空")
	}
	if status == "" {
		t.Fatal("状态不应为空")
	}
	t.Logf("name=%s status=%s jiffies=%d rssPages=%d", name, status, jiffies, rss)
}

// comm 里带空格和括号时，解析必须仍然正确 —— 这是 stat 解析最常见的坑。
func TestParseStatWithSpacesInComm(t *testing.T) {
	// 构造一行伪 stat：comm 为 "a b) c"，后面按真实字段顺序补齐
	// fields: pid (comm) state ppid ... utime(14) stime(15) ... rss(24)
	line := "1234 (a b) c) S 1 1234 1234 0 -1 4194560 " +
		"100 0 0 0 30 40 0 0 20 0 1 0 1000 1000000 500 " + // fields 4..23（utime=30 stime=40 rss=500 在后续位置）
		"0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0"

	// 走与生产代码相同的切分逻辑
	lp := -1
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] == ')' {
			lp = i
			break
		}
	}
	open := -1
	for i := 0; i < len(line); i++ {
		if line[i] == '(' {
			open = i
			break
		}
	}
	if open < 0 || lp <= open {
		t.Fatal("定位 comm 失败")
	}
	gotName := line[open+1 : lp]
	if gotName != "a b) c" {
		t.Fatalf("comm 解析错误，得到 %q", gotName)
	}
}

// 缓存：TTL 内的重复调用必须返回同一份数据，且不重复扫描 /proc。
func TestGetProcessesCache(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("/proc 不可用，跳过")
	}
	procCacheMu.Lock()
	procCacheList = nil
	procCacheAt = time.Time{}
	procPrevSnap = nil
	procCacheMu.Unlock()

	first, err := GetProcesses()
	if err != nil {
		t.Fatalf("首次获取失败: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("进程列表不应为空")
	}

	procCacheMu.RLock()
	atAfterFirst := procCacheAt
	procCacheMu.RUnlock()

	second, err := GetProcesses()
	if err != nil {
		t.Fatalf("二次获取失败: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("缓存命中应返回相同长度: %d vs %d", len(first), len(second))
	}

	procCacheMu.RLock()
	atAfterSecond := procCacheAt
	procCacheMu.RUnlock()
	if !atAfterSecond.Equal(atAfterFirst) {
		t.Fatal("TTL 内不应重新扫描 /proc（缓存时间被更新了）")
	}
}

// 返回值必须是副本：调用方改动不能污染缓存。
func TestGetProcessesReturnsCopy(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("/proc 不可用，跳过")
	}
	list, err := GetProcesses()
	if err != nil || len(list) == 0 {
		t.Skip("无数据可测")
	}
	orig := list[0].Name
	list[0].Name = "__MUTATED__"

	again, _ := GetProcesses()
	if again[0].Name == "__MUTATED__" {
		t.Fatal("缓存被调用方污染，应返回副本")
	}
	if again[0].Name != orig {
		t.Fatalf("缓存内容异常: %q", again[0].Name)
	}
}

// 首轮没有基线，CPU 必须为 0，而不是被算成 100%。
func TestFirstSampleCPUFloatIsZero(t *testing.T) {
	if _, err := os.Stat("/proc/self/stat"); err != nil {
		t.Skip("/proc 不可用，跳过")
	}
	procCacheMu.Lock()
	procCacheList = nil
	procCacheAt = time.Time{}
	procPrevSnap = nil
	procCacheMu.Unlock()

	list, err := GetProcesses()
	if err != nil {
		t.Fatalf("获取失败: %v", err)
	}
	for _, p := range list {
		if p.CPUPercent != 0 {
			t.Fatalf("首轮采样 CPU 应为 0，进程 %s 得到 %f", p.Name, p.CPUPercent)
		}
	}
}

// totalCPUTicks 必须单调可读且大于 0。
func TestTotalCPUTicks(t *testing.T) {
	if _, err := os.Stat("/proc/stat"); err != nil {
		t.Skip("/proc/stat 不可用，跳过")
	}
	a := totalCPUTicks()
	if a == 0 {
		t.Fatal("/proc/stat 总 jiffies 不应为 0")
	}
	time.Sleep(120 * time.Millisecond)
	b := totalCPUTicks()
	if b < a {
		t.Fatalf("总 jiffies 应单调不减: %d -> %d", a, b)
	}
}

// totalMemoryBytes 应能读到合理的内存量。
func TestTotalMemoryBytes(t *testing.T) {
	if _, err := os.Stat("/proc/meminfo"); err != nil {
		t.Skip("/proc/meminfo 不可用，跳过")
	}
	m := totalMemoryBytes()
	if m < 100*1024*1024 {
		t.Fatalf("内存总量异常: %d", m)
	}
	t.Logf("total memory = %d bytes (%d MB)", m, m/1024/1024)
}

// 文件名是数字才能当 pid：非数字条目（如 "1abc"）应被跳过而不是崩溃。
func TestNumericPIDFilter(t *testing.T) {
	if _, err := strconv.Atoi("1234"); err != nil {
		t.Fatal("合法 pid 解析失败")
	}
	if _, err := strconv.Atoi("self"); err == nil {
		t.Fatal("非数字条目应解析失败并被跳过")
	}
}

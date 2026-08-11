package service

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
	syscmd "github.com/minipanel/minipanel/internal/utils/cmd"
)

type FirewallService struct {
	repo *repository.FirewallRepository
}

func NewFirewallService() *FirewallService {
	return &FirewallService{repo: repository.NewFirewallRepository(global.DB)}
}

// isAndroidEnv 检测是否在 Android chroot/容器环境中运行。
// 通过检查 /proc/1/root/system/bin/iptables 是否存在来判断。
// /proc/1/root/ 指向 Android init 进程的根目录，只有 root 权限可读。
func isAndroidEnv() bool {
	if global.IsAndroidChroot {
		return true
	}
	if _, err := os.Stat("/proc/1/root/system/bin/iptables"); err == nil {
		return true
	}
	if findAndroidProcessPID("magiskd") > 0 {
		return true
	}
	if _, err := os.Stat("/opt/minipanel/bin/iptables"); err == nil {
		return true
	}
	if _, err := os.Stat("/opt/minipanel/bin/iptables-android"); err == nil {
		return true
	}
	return false
}

// iptablesBase 返回调用 iptables 的命令前缀。
// Android chroot 环境：通过 /proc/1/root/ 逃逸调用 Android 原生 iptables（legacy 后端）
// 普通 Linux 环境：直接调用系统 iptables
func iptablesBase() []string {
	if !isAndroidEnv() {
		return []string{"iptables"}
	}
	return resolveAndroidIptablesPrefix("iptables")
}

// ip6tablesBase 返回调用 ip6tables 的命令前缀（IPv6）。
func ip6tablesBase() []string {
	if !isAndroidEnv() {
		return []string{"ip6tables"}
	}
	return resolveAndroidIptablesPrefix("ip6tables")
}

// iptablesCmd 创建一个 iptables 命令，自动选择 Android 原生或系统 iptables。
func iptablesCmd(args ...string) *exec.Cmd {
	base := iptablesBase()
	return exec.Command(base[0], append(base[1:], args...)...)
}

// ip6tablesCmd 创建 ip6tables 命令（IPv6），自动选择 Android 原生或系统命令
func ip6tablesCmd(args ...string) *exec.Cmd {
	base := ip6tablesBase()
	return exec.Command(base[0], append(base[1:], args...)...)
}

const androidPrefixCacheTTL = 30 * time.Second

type androidPrefixCacheEntry struct {
	prefix []string
	at     time.Time
}

var (
	androidPrefixCacheMu        sync.Mutex
	androidPrefixCache          = make(map[string]androidPrefixCacheEntry)
	resolvedAndroidIptablesPath string
)

// findAndroidProcessPID 在共享的 /proc 中查找 Android 原生进程（如 magiskd）的 PID，
// 用于构造 /proc/<pid>/root 逃逸路径
func findAndroidProcessPID(name string) int {
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
			cmdline := strings.ReplaceAll(string(data), "\x00", " ")
			if strings.Contains(cmdline, name) {
				pid, _ := strconv.Atoi(pidStr)
				if pid > 0 {
					return pid
				}
			}
		}
	}
	return 0
}

// androidRootCandidates 返回可访问 Android 根目录的候选路径：
// magiskd 进程优先（文档推荐的逃逸入口），其次为 init（/proc/1/root）
func androidRootCandidates() []string {
	var roots []string
	if pid := findAndroidProcessPID("magiskd"); pid > 0 {
		roots = append(roots, fmt.Sprintf("/proc/%d/root", pid))
	}
	roots = append(roots, "/proc/1/root")
	return roots
}

// androidIptablesCandidates 构造按优先级排列的 Android iptables 调用前缀：
// 1. /opt/minipanel/bin/iptables 包装脚本（文档 B2）
// 2. /opt/minipanel/bin/iptables-android 复制二进制（文档 B1）
// 3. chroot 到逃逸根目录后调用（文档实测有效，/proc 挂载为 noexec 时直接执行会失败）
// 4. 逃逸路径直接调用（部分环境可用）
// 5. chroot 内系统命令兜底
func androidIptablesCandidates(bin string, roots []string) [][]string {
	var candidates [][]string
	if bin == "iptables" {
		for _, p := range []string{"/opt/minipanel/bin/iptables", "/opt/minipanel/bin/iptables-android"} {
			if _, err := os.Stat(p); err == nil {
				candidates = append(candidates, []string{p})
			}
		}
	}
	for _, root := range roots {
		androidBin := root + "/system/bin/" + bin
		candidates = append(candidates, []string{"chroot", root + "/", "/system/bin/" + bin})
		candidates = append(candidates, []string{androidBin})
	}
	candidates = append(candidates, []string{bin})
	return candidates
}

// probeIptablesPrefix 通过 -L -n 探测候选命令前缀是否真正可用
func probeIptablesPrefix(prefix []string) bool {
	if len(prefix) == 0 {
		return false
	}
	cmd := exec.Command(prefix[0], append(prefix[1:], "-L", "-n")...)
	if err := cmd.Run(); err != nil {
		global.LOG.Debugf("[Firewall] probe iptables prefix %v failed: %v", prefix, err)
		return false
	}
	return true
}

// resolveAndroidIptablesPrefix 解析可用的 Android iptables/ip6tables 调用前缀并做短时缓存，
// 避免每条规则都重复探测；全部失败时回退到 chroot 内系统命令
func resolveAndroidIptablesPrefix(bin string) []string {
	androidPrefixCacheMu.Lock()
	defer androidPrefixCacheMu.Unlock()

	if entry, ok := androidPrefixCache[bin]; ok && time.Since(entry.at) < androidPrefixCacheTTL {
		return entry.prefix
	}

	candidates := androidIptablesCandidates(bin, androidRootCandidates())
	resolved := []string{bin}
	for _, prefix := range candidates {
		if probeIptablesPrefix(prefix) {
			resolved = prefix
			break
		}
	}
	if len(resolved) > 1 || resolved[0] != bin {
		resolvedAndroidIptablesPath = strings.Join(resolved, " ")
	}
	androidPrefixCache[bin] = androidPrefixCacheEntry{prefix: resolved, at: time.Now()}
	return resolved
}

var (
	firewallChainRe = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}$`)
	firewallSpecRe  = regexp.MustCompile(`^[A-Za-z0-9_\-.:,/*!\[\]]{1,256}$`)
)

// validateFirewallPort 校验端口表达式：单个端口、a-b 区间、逗号分隔多端口。
func validateFirewallPort(port string) error {
	port = strings.TrimSpace(port)
	if port == "" {
		return fmt.Errorf("端口不能为空")
	}
	if !regexp.MustCompile(`^[0-9,\- ]+$`).MatchString(port) {
		return fmt.Errorf("端口格式非法")
	}
	for _, part := range strings.Split(port, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			segs := strings.Split(part, "-")
			if len(segs) != 2 {
				return fmt.Errorf("端口区间格式非法: %s", part)
			}
			lo, err1 := strconv.Atoi(strings.TrimSpace(segs[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(segs[1]))
			if err1 != nil || err2 != nil || lo < 1 || hi > 65535 || lo > hi {
				return fmt.Errorf("端口区间非法: %s", part)
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("端口非法: %s", part)
		}
	}
	return nil
}

func validateFirewallRule(rule *model.FirewallRule) error {
	switch rule.Type {
	case "port", "ip", "dnat":
	default:
		return fmt.Errorf("规则类型非法: %s", rule.Type)
	}
	switch rule.Action {
	case "allow", "deny":
	default:
		return fmt.Errorf("动作非法: %s", rule.Action)
	}
	switch rule.Direction {
	case "in", "out":
	default:
		return fmt.Errorf("方向非法: %s", rule.Direction)
	}
	switch rule.Protocol {
	case "tcp", "udp", "all":
	default:
		return fmt.Errorf("协议非法: %s", rule.Protocol)
	}
	if rule.Type == "port" || rule.Type == "dnat" {
		if err := validateFirewallPort(rule.Port); err != nil {
			return err
		}
	}
	if rule.Type == "ip" || rule.Type == "dnat" {
		isV4, isV6 := ipFamily(rule.IP)
		if !isV4 && !isV6 {
			return fmt.Errorf("IP 地址非法: %s", rule.IP)
		}
	}
	if rule.Type == "dnat" {
		if rule.TargetPort == "" {
			return fmt.Errorf("DNAT 需要目标端口")
		}
		if n, err := strconv.Atoi(strings.TrimSpace(rule.TargetPort)); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("DNAT 目标端口非法")
		}
		if rule.Chain != "" && !firewallChainRe.MatchString(rule.Chain) {
			return fmt.Errorf("DNAT 链名非法")
		}
	}
	return nil
}

// shq 对写入 Magisk 开机脚本的值做单引号转义，防止命令注入。
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (s *FirewallService) Create(item *model.FirewallRule) error {
	if item.Protocol == "" {
		item.Protocol = "tcp"
	}
	if item.Direction == "" {
		item.Direction = "in"
	}
	if item.Action == "" {
		item.Action = "allow"
	}
	if err := validateFirewallRule(item); err != nil {
		return err
	}
	if err := s.repo.Create(item); err != nil {
		return err
	}
	s.syncAndroidPersistence()
	return nil
}

func (s *FirewallService) List() ([]model.FirewallRule, error) {
	return s.repo.List()
}

func (s *FirewallService) Update(item *model.FirewallRule) error {
	if err := validateFirewallRule(item); err != nil {
		return err
	}
	old, err := s.repo.GetByID(item.ID)
	if err == nil && old.Type == "dnat" && (item.Type != "dnat" || !item.Enabled) {
		s.deleteDnatRule(old)
	}
	if err := s.repo.Update(item); err != nil {
		return err
	}
	s.syncAndroidPersistence()
	return nil
}

func (s *FirewallService) Delete(id uint) error {
	rule, err := s.repo.GetByID(id)
	if err == nil && rule.Type == "dnat" {
		s.deleteDnatRule(rule)
	}
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.syncAndroidPersistence()
	return nil
}

// ListDeletedRules 列出已软删除的规则（回收站）
func (s *FirewallService) ListDeletedRules() ([]model.FirewallRule, error) {
	return s.repo.ListDeleted()
}

// RestoreRule 恢复被软删除的规则
func (s *FirewallService) RestoreRule(id uint) error {
	return s.repo.Restore(id)
}

// ClearDeletedRules 一键清空已删除规则（永久删除，不可恢复）
func (s *FirewallService) ClearDeletedRules() (int64, error) {
	return s.repo.ClearDeleted()
}

func (s *FirewallService) getAvailableBackends() []string {
	var backends []string

	// 最高优先级：Android chroot 环境下的原生 iptables（legacy 后端）
	// Ubuntu 的 iptables（nft 后端）与 Android 内核 netfilter ABI 不兼容，
	// 必须通过 /proc/1/root/ 逃逸调用 Android 原生 iptables
	if isAndroidEnv() {
		backends = append(backends, "android-iptables")
		global.LOG.Infof("[Firewall] 检测到 Android chroot 环境，使用 Android 原生 iptables")
	}

	// 优先级: nftables > iptables > firewalld > ufw
	// 只要二进制存在就加入列表，实际启动时再逐个尝试是否可用
	// 不做预测试，因为空规则集/未初始化状态下预测试可能返回错误导致误判

	// nftables直接操作内核规则集，在精简内核/嵌入式环境兼容性最好
	if syscmd.Which("nft") {
		backends = append(backends, "nftables")
	}

	// iptables 直接操作，比ufw依赖少
	if syscmd.Which("iptables") {
		backends = append(backends, "iptables")
	}

	// firewalld 如果正在运行则优先使用
	if syscmd.Which("firewall-cmd") {
		out, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			// 已经在运行，放最前面
			backends = append([]string{"firewalld"}, backends...)
		} else {
			// 没运行放后面
			backends = append(backends, "firewalld")
		}
	}

	// ufw放最后，依赖最多，在精简内核下经常无法使用
	if syscmd.Which("ufw") {
		backends = append(backends, "ufw")
	}

	if len(backends) == 0 {
		return []string{"none"}
	}
	return backends
}

func (s *FirewallService) getFirewallBackend() string {
	backends := s.getAvailableBackends()
	return backends[0]
}

func (s *FirewallService) GetStatus() (map[string]interface{}, error) {
	backends := s.getAvailableBackends()
	backend := backends[0]
	result := map[string]interface{}{
		"backend":            backend,
		"name":               backend,
		"available_backends": backends,
		"installed":          backend != "none",
		"running":            false,
	}

	// 平台检查（非 Linux 时直接给出明确说明）
	if runtime.GOOS != "linux" {
		result["running"] = false
		result["installed"] = false
		result["platform_unsupported"] = true
		result["message"] = fmt.Sprintf("防火墙管理仅支持 Linux 系统，当前系统为 %s", runtime.GOOS)
		result["diagnosis"] = "MiniPanel 防火墙功能依赖 Linux 内核的 netfilter 子系统，Windows/macOS 无法使用"
		return result, nil
	}

	// backend == none：内核或软件缺失，给出诊断
	if backend == "none" {
		result["running"] = false
		result["installed"] = false
		result["message"] = "未检测到任何防火墙后端"
		result["diagnosis"] = s.diagnoseNoBackend()
		return result, nil
	}

	switch backend {
	case "firewalld":
		out, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			result["running"] = true
		}
		if verOut, err := exec.Command("firewall-cmd", "--version").CombinedOutput(); err == nil {
			result["version"] = strings.TrimSpace(string(verOut))
		}
	case "ufw":
		out, err := exec.Command("ufw", "status").CombinedOutput()
		if err == nil && strings.Contains(string(out), "Status: active") {
			result["running"] = true
		}
		// 检查内核模块
		if modErr := s.checkKernelModule("ufw"); modErr != "" {
			result["kernel_warning"] = modErr
		}
	case "nftables":
		out, err := exec.Command("nft", "list", "ruleset").CombinedOutput()
		if err == nil {
			outStr := strings.TrimSpace(string(out))
			result["running"] = len(outStr) > 0
			if strings.Contains(outStr, "table inet filter") {
				result["running"] = true
			}
		} else {
			// nft 命令存在但执行失败，可能是内核 netfilter 子系统不可用
			result["kernel_warning"] = fmt.Sprintf("nft 命令执行失败: %s（可能内核 netfilter 模块不可用）", strings.TrimSpace(string(out)))
		}
	case "android-iptables", "iptables":
		out, err := iptablesCmd("-L", "-n").CombinedOutput()
		result["running"] = err == nil && len(out) > 0
		if err != nil {
			errStr := strings.TrimSpace(string(out))
			if strings.Contains(errStr, "Permission denied") || strings.Contains(err.Error(), "permission") {
				result["kernel_warning"] = "iptables 执行权限不足，请确保 MiniPanel 以 root 运行"
			} else if strings.Contains(errStr, "not found") || strings.Contains(errStr, "no chain") {
				result["kernel_warning"] = "iptables 内核模块未加载，请执行: modprobe ip_tables"
			} else {
				result["kernel_warning"] = fmt.Sprintf("iptables 执行失败: %s（可能内核 netfilter 不可用）", errStr)
			}
		}
		if backend == "android-iptables" {
			result["version"] = "Android iptables (legacy)"
			result["android_env"] = true
			if resolvedAndroidIptablesPath != "" {
				result["resolved_path"] = resolvedAndroidIptablesPath
			}
		}
		v6out, v6err := ip6tablesCmd("-L", "-n").CombinedOutput()
		result["ipv6_supported"] = v6err == nil
		result["ipv6_running"] = v6err == nil && len(v6out) > 0
	}
	return result, nil
}

// diagnoseNoBackend 诊断为什么没有可用后端
func (s *FirewallService) diagnoseNoBackend() string {
	var reasons []string

	// 平台检查放在最前，非 Linux 直接给出明确结论
	if runtime.GOOS != "linux" {
		reasons = append(reasons, fmt.Sprintf("当前系统为 %s，防火墙管理仅支持 Linux", runtime.GOOS))
		reasons = append(reasons, "MiniPanel 防火墙功能依赖 Linux 内核的 netfilter 子系统，Windows/macOS 内核不具备该子系统，无法使用")
		return strings.Join(reasons, "\n")
	}

	// 检查常见后端是否安装
	missingTools := []string{}
	if !syscmd.Which("nft") {
		missingTools = append(missingTools, "nft")
	}
	if !syscmd.Which("iptables") {
		missingTools = append(missingTools, "iptables")
	}
	if !syscmd.Which("firewall-cmd") {
		missingTools = append(missingTools, "firewall-cmd (firewalld)")
	}
	if !syscmd.Which("ufw") {
		missingTools = append(missingTools, "ufw")
	}

	if len(missingTools) == 4 {
		reasons = append(reasons, "未安装任何防火墙管理工具 (nftables/iptables/firewalld/ufw)")
		reasons = append(reasons, "建议安装:")
		reasons = append(reasons, "  Debian/Ubuntu: apt install -y nftables iptables ufw")
		reasons = append(reasons, "  CentOS/RHEL: yum install -y nftables iptables firewalld")
	} else if len(missingTools) > 0 {
		reasons = append(reasons, fmt.Sprintf("部分工具未安装: %s", strings.Join(missingTools, ", ")))
	}

	// 检查内核模块
	if syscmd.Which("iptables") {
		if _, err := iptablesCmd("-L", "-n").CombinedOutput(); err != nil {
			reasons = append(reasons, "iptables 命令存在但执行失败，可能内核 netfilter 模块未加载")
			reasons = append(reasons, "  请尝试: modprobe ip_tables && modprobe iptable_filter")
		}
	}
	if syscmd.Which("nft") {
		if _, err := exec.Command("nft", "list", "ruleset").CombinedOutput(); err != nil {
			reasons = append(reasons, "nft 命令存在但执行失败，可能内核 nf_tables 模块未加载")
			reasons = append(reasons, "  请尝试: modprobe nf_tables")
		}
	}

	// 检查是否在容器中运行（共享内核场景）
	if _, err := os.Stat("/.dockerenv"); err == nil {
		reasons = append(reasons, "检测到在 Docker 容器中运行，容器通常共享宿主机内核，netfilter 操作可能受限")
	}
	if _, err := os.Stat("/proc/1/cgroup"); err == nil {
		if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
			cgroupStr := string(data)
			if strings.Contains(cgroupStr, "lxc") || strings.Contains(cgroupStr, "container") {
				if !strings.Contains(strings.Join(reasons, "\n"), "容器") {
					reasons = append(reasons, "检测到在 LXC/容器环境中运行，netfilter 操作可能被宿主机限制")
				}
			}
		}
	}

	// 检查权限
	if os.Geteuid() != 0 {
		reasons = append(reasons, fmt.Sprintf("MiniPanel 当前以非 root 用户运行 (uid=%d)，防火墙操作需要 root 权限", os.Geteuid()))
	}

	if len(reasons) == 0 {
		return "无法确定具体原因，请检查系统日志或联系支持"
	}
	return strings.Join(reasons, "\n")
}

// checkKernelModule 检查后端依赖的内核模块是否可用
func (s *FirewallService) checkKernelModule(backend string) string {
	switch backend {
	case "ufw":
		// ufw 实际使用 iptables，检查 ip_tables
		out, err := exec.Command("modprobe", "-n", "ip_tables").CombinedOutput()
		if err != nil {
			return fmt.Sprintf("ip_tables 内核模块不可用: %s", strings.TrimSpace(string(out)))
		}
	case "android-iptables", "iptables":
		// Android 环境下内核模块已内置，无需 modprobe
		if isAndroidEnv() {
			return ""
		}
		out, err := exec.Command("modprobe", "-n", "ip_tables").CombinedOutput()
		if err != nil {
			return fmt.Sprintf("ip_tables 内核模块不可用: %s", strings.TrimSpace(string(out)))
		}
	case "nftables":
		out, err := exec.Command("modprobe", "-n", "nf_tables").CombinedOutput()
		if err != nil {
			return fmt.Sprintf("nf_tables 内核模块不可用: %s", strings.TrimSpace(string(out)))
		}
	}
	return ""
}

// Diagnose 一键诊断：返回完整的防火墙环境诊断报告
func (s *FirewallService) Diagnose() map[string]interface{} {
	report := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
	}

	// 平台检查
	report["platform"] = runtime.GOOS
	report["platform_supported"] = runtime.GOOS == "linux"

	if runtime.GOOS != "linux" {
		report["summary"] = fmt.Sprintf("当前系统 %s 不支持防火墙管理，仅 Linux 可用", runtime.GOOS)
		report["recommendation"] = "请在 Linux 服务器上部署 MiniPanel 以使用防火墙功能"
		return report
	}

	// 权限检查
	uid := os.Geteuid()
	report["uid"] = uid
	report["is_root"] = uid == 0

	// 容器检测
	isContainer := false
	if _, err := os.Stat("/.dockerenv"); err == nil {
		isContainer = true
		report["container_type"] = "docker"
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		cgroupStr := string(data)
		if strings.Contains(cgroupStr, "lxc") {
			isContainer = true
			report["container_type"] = "lxc"
		}
	}
	report["in_container"] = isContainer

	// Android 环境检测
	androidEnv := isAndroidEnv()
	report["android_env"] = androidEnv
	report["android_magiskd_pid"] = findAndroidProcessPID("magiskd")
	if androidEnv {
		report["container_type"] = "android-chroot"
		report["in_container"] = true
		if resolvedAndroidIptablesPath != "" {
			report["android_iptables_resolved"] = resolvedAndroidIptablesPath
		}
	}

	// 后端工具检测
	backends := s.getAvailableBackends()
	report["available_backends"] = backends
	report["tools_installed"] = map[string]bool{
		"nft":              syscmd.Which("nft"),
		"iptables":         syscmd.Which("iptables"),
		"firewall-cmd":     syscmd.Which("firewall-cmd"),
		"ufw":              syscmd.Which("ufw"),
		"android-iptables": androidEnv,
	}

	// 内核模块检测
	kernelModules := map[string]string{}
	if syscmd.Which("iptables") {
		if out, err := iptablesCmd("-L", "-n").CombinedOutput(); err != nil {
			kernelModules["ip_tables"] = fmt.Sprintf("unavailable: %s", strings.TrimSpace(string(out)))
		} else {
			kernelModules["ip_tables"] = "ok"
		}
	}
	if syscmd.Which("nft") {
		if out, err := exec.Command("nft", "list", "ruleset").CombinedOutput(); err != nil {
			kernelModules["nf_tables"] = fmt.Sprintf("unavailable: %s", strings.TrimSpace(string(out)))
		} else {
			kernelModules["nf_tables"] = "ok"
		}
	}
	report["kernel_modules"] = kernelModules

	// IPv6 支持检测（ip6tables 是否可用/是否有规则）
	v6out, v6err := ip6tablesCmd("-L", "-n").CombinedOutput()
	report["ipv6_supported"] = v6err == nil
	if v6err != nil {
		report["ipv6_error"] = strings.TrimSpace(string(v6out))
	} else {
		report["ipv6_rules"] = len(v6out) > 0
	}

	// 总结
	if backends[0] == "none" {
		report["summary"] = "无法使用防火墙功能"
		report["recommendation"] = s.diagnoseNoBackend()
	} else if isRoot, ok := report["is_root"].(bool); !ok || !isRoot {
		report["summary"] = "防火墙工具已安装，但 MiniPanel 未以 root 运行，操作将被拒绝"
		report["recommendation"] = "请以 root 用户或 sudo 启动 MiniPanel"
	} else if androidEnv {
		report["summary"] = "Android chroot 环境检测到，使用 Android 原生 iptables (legacy)"
		report["recommendation"] = "通过 /proc/1/root/ 逃逸调用 Android 原生 iptables，与 Android 内核 netfilter 完全兼容。注意：规则重启后丢失，建议使用 Magisk service.d 持久化。"
	} else if isContainer {
		report["summary"] = "在容器环境中运行，netfilter 操作可能受限"
		report["recommendation"] = "建议在宿主机上使用防火墙功能，或在容器启动时加 --privileged 参数"
	} else {
		report["summary"] = fmt.Sprintf("防火墙后端可用: %s", strings.Join(backends, ", "))
		report["recommendation"] = "环境正常，可正常使用防火墙功能"
	}

	return report
}

func (s *FirewallService) Start() error {
	backends := s.getAvailableBackends()
	global.LOG.Infof("[Firewall] available backends: %v", backends)
	if backends[0] == "none" {
		diagnosis := s.diagnoseNoBackend()
		return fmt.Errorf("未检测到可用的防火墙后端\n\n诊断信息:\n%s", diagnosis)
	}

	// 收集所有后端的失败原因，便于排查
	var failures []string
	var lastErr error
	for _, backend := range backends {
		global.LOG.Infof("[Firewall] trying to start with backend: %s", backend)
		var err error
		switch backend {
		case "firewalld":
			if out, startErr := exec.Command("systemctl", "start", "firewalld").CombinedOutput(); startErr != nil {
				if out2, err2 := exec.Command("service", "firewalld", "start").CombinedOutput(); err2 != nil {
					err = fmt.Errorf("systemctl: %s | service: %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
				} else {
					err = nil
				}
			} else {
				err = nil
			}
		case "ufw":
			cmd := exec.Command("ufw", "--force", "enable")
			if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
				outStr := strings.TrimSpace(string(out))
				// 如果是内核模块缺失，直接跳过这个后端
				if strings.Contains(outStr, "missing kernel module") ||
					strings.Contains(outStr, "Could not fetch rule set") ||
					strings.Contains(outStr, "Problem running") ||
					strings.Contains(outStr, "couldn't load") ||
					strings.Contains(outStr, "invalid port/service") {
					lastErr = fmt.Errorf("ufw: %s", outStr)
					failures = append(failures, fmt.Sprintf("ufw: %s", outStr))
					global.LOG.Warnf("[Firewall] ufw not usable, trying next backend: %v", lastErr)
					continue
				}
				err = fmt.Errorf("ufw: %s", outStr)
			}
		case "nftables":
			// nftables需要先确保基础表存在，再应用规则
			if ensureErr := s.ensureNftablesBase(); ensureErr != nil {
				global.LOG.Warnf("[Firewall] ensure nftables base failed: %v", ensureErr)
				err = ensureErr
			} else {
				_, err = s.ApplyRulesForBackend(backend)
			}
		case "android-iptables", "iptables":
			_, err = s.ApplyRulesForBackend(backend)
		}

		if err == nil {
			// 再次验证后端是否真的可用
			if s.checkBackendRunning(backend) {
				global.LOG.Infof("[Firewall] successfully started with backend: %s", backend)
				return nil
			}
			global.LOG.Warnf("[Firewall] backend %s returned success but not running, trying next", backend)
			lastErr = fmt.Errorf("%s 启动后未检测到运行状态", backend)
			failures = append(failures, lastErr.Error())
		} else {
			lastErr = err
			failures = append(failures, fmt.Sprintf("%s: %v", backend, err))
			global.LOG.Warnf("[Firewall] backend %s failed: %v, trying next", backend, err)
		}
	}

	// 所有后端都失败，返回完整失败链 + 诊断
	diagnosis := s.diagnoseNoBackend()
	return fmt.Errorf("所有防火墙后端启动均失败\n\n尝试的后端及失败原因:\n%s\n\n环境诊断:\n%s",
		strings.Join(failures, "\n"), diagnosis)
}

func (s *FirewallService) checkBackendRunning(backend string) bool {
	switch backend {
	case "firewalld":
		out, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
		return err == nil && strings.TrimSpace(string(out)) == "running"
	case "ufw":
		out, err := exec.Command("ufw", "status").CombinedOutput()
		return err == nil && strings.Contains(string(out), "Status: active")
	case "nftables":
		out, err := exec.Command("nft", "list", "ruleset").CombinedOutput()
		return err == nil && strings.Contains(string(out), "table inet filter")
	case "android-iptables", "iptables":
		out, err := iptablesCmd("-L", "-n").CombinedOutput()
		return err == nil && len(out) > 0
	}
	return false
}

func (s *FirewallService) Stop() error {
	backends := s.getAvailableBackends()
	for _, backend := range backends {
		switch backend {
		case "firewalld":
			exec.Command("systemctl", "stop", "firewalld").Run()
			exec.Command("service", "firewalld", "stop").Run()
		case "ufw":
			cmd := exec.Command("ufw", "--force", "disable")
			cmd.Run()
		case "nftables":
			exec.Command("nft", "flush", "ruleset").Run()
		case "android-iptables", "iptables":
			chains := []string{"INPUT", "OUTPUT", "FORWARD"}
			for _, chain := range chains {
				iptablesCmd("-F", chain).Run()
				ip6tablesCmd("-F", chain).Run()
			}
			iptablesCmd("-P", "INPUT", "ACCEPT").Run()
			ip6tablesCmd("-P", "INPUT", "ACCEPT").Run()
			iptablesCmd("-P", "OUTPUT", "ACCEPT").Run()
			ip6tablesCmd("-P", "OUTPUT", "ACCEPT").Run()
			iptablesCmd("-P", "FORWARD", "ACCEPT").Run()
			ip6tablesCmd("-P", "FORWARD", "ACCEPT").Run()
		}
	}
	return nil
}

func (s *FirewallService) flushRulesForBackend(backend string) error {
	switch backend {
	case "firewalld":
		exec.Command("firewall-cmd", "--reload").Run()
	case "ufw":
		cmd := exec.Command("ufw", "--force", "reset")
		cmd.Stdin = strings.NewReader("y\n")
		cmd.Run()
	case "nftables":
		exec.Command("nft", "flush", "ruleset").Run()
		s.ensureNftablesBase()
	case "android-iptables", "iptables":
		chains := []string{"INPUT", "OUTPUT", "FORWARD"}
		for _, chain := range chains {
			iptablesCmd("-F", chain).Run()
			ip6tablesCmd("-F", chain).Run()
		}
		iptablesCmd("-P", "INPUT", "ACCEPT").Run()
		ip6tablesCmd("-P", "INPUT", "ACCEPT").Run()
		iptablesCmd("-P", "OUTPUT", "ACCEPT").Run()
		ip6tablesCmd("-P", "OUTPUT", "ACCEPT").Run()
		iptablesCmd("-P", "FORWARD", "ACCEPT").Run()
		ip6tablesCmd("-P", "FORWARD", "ACCEPT").Run()
	}
	return nil
}

func (s *FirewallService) ensureNftablesBase() error {
	commands := []string{
		"add table inet filter",
		"add chain inet filter input { type filter hook input priority 0; policy accept; }",
		"add chain inet filter output { type filter hook output priority 0; policy accept; }",
		"add chain inet filter forward { type filter hook forward priority 0; policy accept; }",
	}
	for _, cmdStr := range commands {
		args := strings.Fields(cmdStr)
		cmd := exec.Command("nft", args...)
		if err := cmd.Run(); err != nil {
			if !strings.Contains(err.Error(), "File exists") {
				global.LOG.Warnf("nft create base failed: %v", err)
			}
		}
	}
	return nil
}

// normalizePort 将前端端口格式(80,443,3306-3308)转换为目标后端格式
// dashBackend: firewalld, nftables (使用 - 表示范围)
// colonBackend: ufw, iptables (使用 : 表示范围)
func normalizePort(port string, useDash bool) string {
	port = strings.ReplaceAll(port, " ", "")
	if useDash {
		return strings.ReplaceAll(port, ":", "-")
	}
	return strings.ReplaceAll(port, "-", ":")
}

func (s *FirewallService) ApplyRules() (string, error) {
	backend := s.getFirewallBackend()
	return s.ApplyRulesForBackend(backend)
}

func (s *FirewallService) ApplyRulesForBackend(backend string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("防火墙管理仅支持Linux系统")
	}
	if backend == "none" {
		return "", fmt.Errorf("未检测到可用的防火墙后端")
	}

	rules, err := s.repo.List()
	if err != nil {
		return "", err
	}

	var output []string
	global.LOG.Infof("[Firewall] applying %d rules using %s backend", len(rules), backend)

	if err := s.flushRulesForBackend(backend); err != nil {
		global.LOG.Warnf("[Firewall] flush rules failed: %v", err)
	}

	if backend == "ufw" {
		exec.Command("ufw", "--force", "default", "deny", "incoming").Run()
		exec.Command("ufw", "--force", "default", "allow", "outgoing").Run()
		// reset后ufw被禁用，先enable让基础框架成功加载，再逐个添加规则
		enableCmd := exec.Command("ufw", "--force", "enable")
		if out, err := enableCmd.CombinedOutput(); err != nil {
			outStr := strings.TrimSpace(string(out))
			if strings.Contains(outStr, "missing kernel module") ||
				strings.Contains(outStr, "Could not fetch rule set") ||
				strings.Contains(outStr, "Problem running") {
				return strings.Join(output, "\n"), fmt.Errorf("ufw内核模块缺失")
			}
			return strings.Join(output, "\n"), fmt.Errorf("ufw enable失败: %s", outStr)
		}
		// ufw enable后会自动加载before.rules，其中已包含回环和已建立连接允许规则
	}

	if backend == "nftables" {
		s.ensureNftablesBase()
	}

	// 添加基础规则：回环接口允许通过
	s.allowLoopbackForBackend(backend)
	// 注意：精简内核可能没有conntrack模块，跳过ESTABLISHED规则，避免依赖缺失
	// 如果需要允许已建立连接可以手动添加，默认我们只开放用户配置的端口即可

	for _, rule := range rules {
		if !rule.Enabled {
			output = append(output, fmt.Sprintf("skip disabled rule %d: %s", rule.ID, rule.Name))
			continue
		}

		var err error
		switch backend {
		case "firewalld":
			err = s.applyFirewalldRule(&rule)
		case "ufw":
			err = s.applyUfwRule(&rule)
		case "nftables":
			err = s.applyNftablesRule(&rule)
		case "android-iptables", "iptables":
			if rule.Type == "dnat" {
				err = s.applyDnatRule(&rule)
				if err == nil {
					if v6err := s.applyIp6DnatRule(&rule); v6err != nil {
						global.LOG.Warnf("[Firewall] rule %d (%s) IPv6 DNAT apply failed: %v", rule.ID, rule.Name, v6err)
						output = append(output, fmt.Sprintf("rule %d (%s) IPv6 DNAT 部分失败: %v", rule.ID, rule.Name, v6err))
					}
				}
			} else {
				err = s.applyIptablesRule(&rule)
				if err == nil {
					// IPv6 同步应用：ip6tables 不可用时仅告警，不影响 IPv4 规则生效
					if v6err := s.applyIp6tablesRule(&rule); v6err != nil {
						global.LOG.Warnf("[Firewall] rule %d (%s) IPv6 apply failed: %v", rule.ID, rule.Name, v6err)
						output = append(output, fmt.Sprintf("rule %d (%s) IPv6 部分失败: %v", rule.ID, rule.Name, v6err))
					}
				}
			}
		default:
			err = fmt.Errorf("不支持的防火墙后端: %s", backend)
		}

		if err != nil {
			output = append(output, fmt.Sprintf("rule %d (%s) failed: %v", rule.ID, rule.Name, err))
			global.LOG.Errorf("[Firewall] rule %d failed: %v", rule.ID, err)
		} else {
			output = append(output, fmt.Sprintf("rule %d (%s) applied", rule.ID, rule.Name))
		}
	}

	if backend == "firewalld" {
		if out, err := exec.Command("firewall-cmd", "--runtime-to-permanent").CombinedOutput(); err != nil {
			global.LOG.Warnf("[Firewall] runtime-to-permanent failed: %s", string(out))
		}
	}

	if backend == "android-iptables" {
		if path := s.syncAndroidPersistence(); path != "" {
			output = append(output, "已写入开机持久化脚本: "+path)
		}
	}

	if len(output) == 0 {
		return "No rules to apply", nil
	}
	return strings.Join(output, "\n"), nil
}

// buildAndroidPersistScript 生成 Magisk service.d 开机脚本内容（文档方案 C）
// 脚本不冲刷 Android 原有规则，仅追加面板启用的规则，并用 -C 检查避免重复
func buildAndroidPersistScript(rules []model.FirewallRule) string {
	var sb strings.Builder
	sb.WriteString("#!/system/bin/sh\n")
	sb.WriteString("# Generated by MiniPanel - firewall persistence (IPv4 + IPv6)\n")
	sb.WriteString("sleep 30\n")
	sb.WriteString("AIPT=/system/bin/iptables\n")
	sb.WriteString("AIPT6=/system/bin/ip6tables\n")
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		chain := "INPUT"
		if rule.Direction == "out" {
			chain = "OUTPUT"
		}
		action := "ACCEPT"
		if rule.Action == "deny" {
			action = "DROP"
		}
		if rule.Type == "port" && rule.Port != "" {
			proto := rule.Protocol
			if proto == "" || proto == "all" {
				proto = "tcp"
			}
			ports := strings.Split(normalizePort(rule.Port, false), ",")
			for _, port := range ports {
				port = strings.TrimSpace(port)
				if port == "" {
					continue
				}
				sb.WriteString(fmt.Sprintf(
					"$AIPT -C %s -p %s --dport %s -j %s 2>/dev/null || $AIPT -A %s -p %s --dport %s -j %s\n",
					shq(chain), shq(proto), shq(port), shq(action), shq(chain), shq(proto), shq(port), shq(action)))
				sb.WriteString(fmt.Sprintf(
					"$AIPT6 -C %s -p %s --dport %s -j %s 2>/dev/null || $AIPT6 -A %s -p %s --dport %s -j %s\n",
					shq(chain), shq(proto), shq(port), shq(action), shq(chain), shq(proto), shq(port), shq(action)))
			}
		} else if rule.Type == "ip" && rule.IP != "" {
			ip := strings.TrimSpace(rule.IP)
			if ip == "" {
				continue
			}
			isV4, isV6 := ipFamily(ip)
			if isV4 {
				sb.WriteString(fmt.Sprintf(
					"$AIPT -C %s -s %s -j %s 2>/dev/null || $AIPT -A %s -s %s -j %s\n",
					shq(chain), shq(ip), shq(action), shq(chain), shq(ip), shq(action)))
			}
			if isV6 {
				sb.WriteString(fmt.Sprintf(
					"$AIPT6 -C %s -s %s -j %s 2>/dev/null || $AIPT6 -A %s -s %s -j %s\n",
					shq(chain), shq(ip), shq(action), shq(chain), shq(ip), shq(action)))
			}
		} else if rule.Type == "dnat" {
			proto := rule.Protocol
			if proto == "" || proto == "all" {
				proto = "tcp"
			}
			publicPort := strings.TrimSpace(rule.Port)
			targetIP := strings.TrimSpace(rule.IP)
			targetPort := strings.TrimSpace(rule.TargetPort)
			if publicPort == "" || targetIP == "" || targetPort == "" {
				continue
			}
			natChain := dnatChain(&rule)
			isV4, isV6 := ipFamily(targetIP)
			if isV4 {
				sb.WriteString(fmt.Sprintf(
					"$AIPT -t nat -N %s 2>/dev/null; $AIPT -t nat -C PREROUTING -j %s 2>/dev/null || $AIPT -t nat -A PREROUTING -j %s 2>/dev/null\n",
					shq(natChain), shq(natChain), shq(natChain)))
				sb.WriteString(fmt.Sprintf(
					"$AIPT -t nat -C %s -p %s --dport %s -j DNAT --to-destination %s:%s 2>/dev/null || $AIPT -t nat -A %s -p %s --dport %s -j DNAT --to-destination %s:%s\n",
					shq(natChain), shq(proto), shq(publicPort), shq(targetIP), shq(targetPort), shq(natChain), shq(proto), shq(publicPort), shq(targetIP), shq(targetPort)))
				if rule.Masq {
					sb.WriteString(fmt.Sprintf(
						"$AIPT -t nat -C POSTROUTING -d %s -j MASQUERADE 2>/dev/null || $AIPT -t nat -A POSTROUTING -d %s -j MASQUERADE\n",
						shq(targetIP), shq(targetIP)))
				}
			}
			if isV6 {
				sb.WriteString(fmt.Sprintf(
					"$AIPT6 -t nat -N %s 2>/dev/null; $AIPT6 -t nat -C PREROUTING -j %s 2>/dev/null || $AIPT6 -t nat -A PREROUTING -j %s 2>/dev/null\n",
					shq(natChain), shq(natChain), shq(natChain)))
				sb.WriteString(fmt.Sprintf(
					"$AIPT6 -t nat -C %s -p %s --dport %s -j DNAT --to-destination [%s]:%s 2>/dev/null || $AIPT6 -t nat -A %s -p %s --dport %s -j DNAT --to-destination [%s]:%s\n",
					shq(natChain), shq(proto), shq(publicPort), shq(targetIP), shq(targetPort), shq(natChain), shq(proto), shq(publicPort), shq(targetIP), shq(targetPort)))
				if rule.Masq {
					sb.WriteString(fmt.Sprintf(
						"$AIPT6 -t nat -C POSTROUTING -d %s -j MASQUERADE 2>/dev/null || $AIPT6 -t nat -A POSTROUTING -d %s -j MASQUERADE\n",
						shq(targetIP), shq(targetIP)))
				}
			}
		}
	}
	return sb.String()
}

// findAndroidServiceDir 通过逃逸路径定位并准备 Magisk service.d 目录
func (s *FirewallService) findAndroidServiceDir() string {
	for _, root := range androidRootCandidates() {
		dir := root + "/data/adb/service.d"
		if err := os.MkdirAll(dir, 0755); err == nil && testWritable(dir) {
			return dir
		}
	}
	return ""
}

// persistAndroidRules 把当前面板规则写入 Magisk 开机脚本，实现 Android 防火墙持久化
func (s *FirewallService) persistAndroidRules(rules []model.FirewallRule) (string, error) {
	if !isAndroidEnv() {
		return "", nil
	}
	serviceDir := s.findAndroidServiceDir()
	if serviceDir == "" {
		return "", fmt.Errorf("未找到可写的 Magisk service.d 目录")
	}
	path := filepath.Join(serviceDir, "98-minipanel-firewall.sh")
	content := buildAndroidPersistScript(rules)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", err
	}
	global.LOG.Infof("[Firewall] Android 规则已持久化: %s", path)
	return path, nil
}

// syncAndroidPersistence 在增删改规则后同步 Magisk 开机脚本；非 Android 环境直接跳过
func (s *FirewallService) syncAndroidPersistence() string {
	if !isAndroidEnv() {
		return ""
	}
	rules, err := s.repo.List()
	if err != nil {
		global.LOG.Warnf("[Firewall] sync persistence: list rules failed: %v", err)
		return ""
	}
	path, err := s.persistAndroidRules(rules)
	if err != nil {
		global.LOG.Warnf("[Firewall] sync persistence failed: %v", err)
		return ""
	}
	return path
}

func (s *FirewallService) allowLoopbackForBackend(backend string) {
	switch backend {
	case "firewalld":
		return
	case "ufw":
		exec.Command("ufw", "--force", "allow", "in", "on", "lo").Run()
		exec.Command("ufw", "--force", "allow", "out", "on", "lo").Run()
	case "nftables":
		exec.Command("nft", "add", "rule", "inet", "filter", "input", "iif", "lo", "accept").Run()
		exec.Command("nft", "add", "rule", "inet", "filter", "output", "oif", "lo", "accept").Run()
		// nftables ct state需要内核支持ct模块，精简内核可能没有，默认不添加
		// 只开放用户指定端口即可满足基本需求
	case "android-iptables", "iptables":
		iptablesCmd("-A", "INPUT", "-i", "lo", "-j", "ACCEPT").Run()
		iptablesCmd("-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT").Run()
		ip6tablesCmd("-A", "INPUT", "-i", "lo", "-j", "ACCEPT").Run()
		ip6tablesCmd("-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT").Run()
		// 不使用-m state，避免依赖conntrack模块，精简内核环境下不需要状态检测
	}
}

func (s *FirewallService) applyFirewalldRule(rule *model.FirewallRule) error {
	if rule.Type == "port" && rule.Port != "" {
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		portSpec := fmt.Sprintf("%s/%s", normalizePort(rule.Port, true), proto)

		var cmd *exec.Cmd
		if rule.Action == "allow" {
			cmd = exec.Command("firewall-cmd", "--add-port", portSpec)
		} else {
			richRule := fmt.Sprintf("rule family=ipv4 port port=%s protocol=%s reject", normalizePort(rule.Port, true), proto)
			cmd = exec.Command("firewall-cmd", "--add-rich-rule", richRule)
		}

		if out, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(out), "ALREADY_ENABLED") {
				return fmt.Errorf("%s", strings.TrimSpace(string(out)))
			}
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		var richRule string
		if rule.Action == "allow" {
			richRule = fmt.Sprintf("rule family=ipv4 source address=%s accept", rule.IP)
		} else {
			richRule = fmt.Sprintf("rule family=ipv4 source address=%s drop", rule.IP)
		}
		cmd := exec.Command("firewall-cmd", "--add-rich-rule", richRule)
		if out, err := cmd.CombinedOutput(); err != nil {
			if !strings.Contains(string(out), "ALREADY_ENABLED") {
				return fmt.Errorf("%s", strings.TrimSpace(string(out)))
			}
		}
	}
	return nil
}

func (s *FirewallService) applyUfwRule(rule *model.FirewallRule) error {
	direction := "in"
	if rule.Direction == "out" {
		direction = "out"
	}
	action := "allow"
	if rule.Action == "deny" {
		action = "deny"
	}

	if rule.Type == "port" && rule.Port != "" {
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		portStr := normalizePort(rule.Port, false)
		// 拆分逗号分隔的多端口为单独规则，避免依赖multiport内核模块
		ports := strings.Split(portStr, ",")
		for _, port := range ports {
			port = strings.TrimSpace(port)
			if port == "" {
				continue
			}
			args := []string{"--force", action, direction, "proto", proto, "to", "any", "port", port}
			cmd := exec.Command("ufw", args...)
			if out, err := cmd.CombinedOutput(); err != nil {
				outStr := strings.TrimSpace(string(out))
				if !strings.Contains(outStr, "Skipping adding existing rule") &&
					!strings.Contains(outStr, "already exists") &&
					!strings.Contains(outStr, "Rules updated") &&
					!strings.Contains(outStr, "Rule added") {
					return fmt.Errorf("端口 %s 添加失败: %s", port, outStr)
				}
			}
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		args := []string{"--force", action, direction, "from", rule.IP}
		cmd := exec.Command("ufw", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			outStr := string(out)
			if !strings.Contains(outStr, "Skipping adding existing rule") &&
				!strings.Contains(outStr, "already exists") &&
				!strings.Contains(outStr, "Rules updated") &&
				!strings.Contains(outStr, "Rule added") {
				return fmt.Errorf("%s", strings.TrimSpace(outStr))
			}
		}
	}
	return nil
}

func (s *FirewallService) applyNftablesRule(rule *model.FirewallRule) error {
	chain := "input"
	if rule.Direction == "out" {
		chain = "output"
	}
	action := "accept"
	if rule.Action == "deny" {
		action = "drop"
	}

	if rule.Type == "port" && rule.Port != "" {
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		port := normalizePort(rule.Port, true)
		var args []string

		if strings.Contains(port, ",") {
			ports := strings.Split(port, ",")
			for i := range ports {
				ports[i] = strings.TrimSpace(ports[i])
			}
			portSet := "{ " + strings.Join(ports, ", ") + " }"
			args = []string{"add", "rule", "inet", "filter", chain, proto, "dport", portSet, action}
		} else {
			args = []string{"add", "rule", "inet", "filter", chain, proto, "dport", port, action}
		}
		cmd := exec.Command("nft", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		args := []string{"add", "rule", "inet", "filter", chain, "ip", "saddr", rule.IP, action}
		cmd := exec.Command("nft", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// ipFamily 判断 IP/IP 段属于 IPv4 还是 IPv6；CIDR 与单地址均支持
func ipFamily(s string) (isV4, isV6 bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return false, false
	}
	if strings.Contains(s, "/") {
		_, ipNet, err := net.ParseCIDR(s)
		if err != nil {
			return false, false
		}
		return ipNet.IP.To4() != nil, ipNet.IP.To4() == nil
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return false, false
	}
	return ip.To4() != nil, ip.To4() == nil
}

func (s *FirewallService) applyIptablesRule(rule *model.FirewallRule) error {
	action := "ACCEPT"
	if rule.Action == "deny" {
		action = "DROP"
	}
	chain := "INPUT"
	if rule.Direction == "out" {
		chain = "OUTPUT"
	}

	if rule.Type == "port" && rule.Port != "" {
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		portStr := normalizePort(rule.Port, false)
		// 拆分逗号分隔多端口为单独规则，避免依赖multiport内核模块
		ports := strings.Split(portStr, ",")
		for _, port := range ports {
			port = strings.TrimSpace(port)
			if port == "" {
				continue
			}
			// 检查是否已存在规则（简单判断，忽略错误继续添加）
			checkCmd := iptablesCmd("-C", chain, "-p", proto, "--dport", port, "-j", action)
			if err := checkCmd.Run(); err != nil {
				addCmd := iptablesCmd("-A", chain, "-p", proto, "--dport", port, "-j", action)
				if out, err := addCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("端口 %s 添加失败: %s", port, strings.TrimSpace(string(out)))
				}
			}
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		isV4, _ := ipFamily(rule.IP)
		if !isV4 {
			return nil // IPv6 地址由 applyIp6tablesRule 处理
		}
		checkCmd := iptablesCmd("-C", chain, "-s", rule.IP, "-j", action)
		if err := checkCmd.Run(); err != nil {
			addCmd := iptablesCmd("-A", chain, "-s", rule.IP, "-j", action)
			if out, err := addCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s", strings.TrimSpace(string(out)))
			}
		}
	}
	return nil
}

func (s *FirewallService) applyIp6tablesRule(rule *model.FirewallRule) error {
	action := "ACCEPT"
	if rule.Action == "deny" {
		action = "DROP"
	}
	chain := "INPUT"
	if rule.Direction == "out" {
		chain = "OUTPUT"
	}

	if rule.Type == "port" && rule.Port != "" {
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		portStr := normalizePort(rule.Port, false)
		ports := strings.Split(portStr, ",")
		for _, port := range ports {
			port = strings.TrimSpace(port)
			if port == "" {
				continue
			}
			checkCmd := ip6tablesCmd("-C", chain, "-p", proto, "--dport", port, "-j", action)
			if err := checkCmd.Run(); err != nil {
				addCmd := ip6tablesCmd("-A", chain, "-p", proto, "--dport", port, "-j", action)
				if out, err := addCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("IPv6 端口 %s 添加失败: %s", port, strings.TrimSpace(string(out)))
				}
			}
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		_, isV6 := ipFamily(rule.IP)
		if !isV6 {
			return nil // IPv4 地址由 applyIptablesRule 处理
		}
		checkCmd := ip6tablesCmd("-C", chain, "-s", rule.IP, "-j", action)
		if err := checkCmd.Run(); err != nil {
			addCmd := ip6tablesCmd("-A", chain, "-s", rule.IP, "-j", action)
			if out, err := addCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s", strings.TrimSpace(string(out)))
			}
		}
	}
	return nil
}

// dnatChain 返回 DNAT 使用的 nat 链，默认 PREROUTING
func dnatChain(rule *model.FirewallRule) string {
	chain := strings.TrimSpace(rule.Chain)
	if chain == "" {
		return "PREROUTING"
	}
	return chain
}

// ensureDnatChain 确保自定义 nat 链存在并从 PREROUTING 跳转（Android oem_nat_pre 场景）
func ensureDnatChain(cmd func(args ...string) *exec.Cmd, chain string) {
	if chain == "PREROUTING" {
		return
	}
	cmd("-t", "nat", "-N", chain).Run()
	cmd("-t", "nat", "-C", "PREROUTING", "-j", chain).Run()
	cmd("-t", "nat", "-A", "PREROUTING", "-j", chain).Run()
}

// applyDnatRule 应用 IPv4 DNAT 端口转发规则
func (s *FirewallService) applyDnatRule(rule *model.FirewallRule) error {
	isV4, _ := ipFamily(rule.IP)
	if !isV4 {
		return nil
	}
	return s.applyDnatWithCmd(rule, iptablesCmd)
}

// applyIp6DnatRule 应用 IPv6 DNAT 端口转发规则
func (s *FirewallService) applyIp6DnatRule(rule *model.FirewallRule) error {
	_, isV6 := ipFamily(rule.IP)
	if !isV6 {
		return nil
	}
	return s.applyDnatWithCmd(rule, ip6tablesCmd)
}

func (s *FirewallService) applyDnatWithCmd(rule *model.FirewallRule, cmd func(args ...string) *exec.Cmd) error {
	chain := dnatChain(rule)
	publicPort := strings.TrimSpace(rule.Port)
	targetIP := strings.TrimSpace(rule.IP)
	targetPort := strings.TrimSpace(rule.TargetPort)
	if chain == "" || publicPort == "" || targetIP == "" || targetPort == "" {
		return fmt.Errorf("DNAT 规则缺少参数")
	}
	proto := rule.Protocol
	if proto == "" || proto == "all" {
		proto = "tcp"
	}
	target := fmt.Sprintf("%s:%s", targetIP, targetPort)
	ensureDnatChain(cmd, chain)

	checkCmd := cmd("-t", "nat", "-C", chain, "-p", proto, "--dport", publicPort, "-j", "DNAT", "--to-destination", target)
	if err := checkCmd.Run(); err != nil {
		addCmd := cmd("-t", "nat", "-A", chain, "-p", proto, "--dport", publicPort, "-j", "DNAT", "--to-destination", target)
		if out, err := addCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("DNAT 规则添加失败: %s", strings.TrimSpace(string(out)))
		}
	}

	if rule.Masq {
		masqCheck := cmd("-t", "nat", "-C", "POSTROUTING", "-d", targetIP, "-j", "MASQUERADE")
		if err := masqCheck.Run(); err != nil {
			masqAdd := cmd("-t", "nat", "-A", "POSTROUTING", "-d", targetIP, "-j", "MASQUERADE")
			if out, err := masqAdd.CombinedOutput(); err != nil {
				global.LOG.Warnf("[Firewall] DNAT 回程 MASQUERADE 添加失败: %s", strings.TrimSpace(string(out)))
			}
		}
	}
	return nil
}

// deleteDnatRule 从 nat 表删除对应的 DNAT / MASQUERADE 规则
func (s *FirewallService) deleteDnatRule(rule *model.FirewallRule) error {
	isV4, isV6 := ipFamily(rule.IP)
	chain := dnatChain(rule)
	proto := rule.Protocol
	if proto == "" || proto == "all" {
		proto = "tcp"
	}
	targetIP := strings.TrimSpace(rule.IP)
	targetPort := strings.TrimSpace(rule.TargetPort)
	publicPort := strings.TrimSpace(rule.Port)
	if targetIP == "" || targetPort == "" || publicPort == "" {
		return nil
	}
	target := fmt.Sprintf("%s:%s", targetIP, targetPort)
	if isV4 {
		iptablesCmd("-t", "nat", "-D", chain, "-p", proto, "--dport", publicPort, "-j", "DNAT", "--to-destination", target).Run()
		iptablesCmd("-t", "nat", "-D", "POSTROUTING", "-d", targetIP, "-j", "MASQUERADE").Run()
	}
	if isV6 {
		ip6tablesCmd("-t", "nat", "-D", chain, "-p", proto, "--dport", publicPort, "-j", "DNAT", "--to-destination", target).Run()
		ip6tablesCmd("-t", "nat", "-D", "POSTROUTING", "-d", targetIP, "-j", "MASQUERADE").Run()
	}
	return nil
}

func init() {
	os.MkdirAll("/etc/nftables", 0755)
}

// LiveRules 实时查看系统 iptables/ip6tables 规则（-L --line-numbers -n）
// chain: INPUT / OUTPUT / FORWARD / 空=all
// family: 空=全部(先 IPv4 后 IPv6), "4"/"ipv4"=仅 IPv4, "6"/"ipv6"=仅 IPv6
// table: 空=filter, "nat"=nat 表（用于查看 DNAT 端口转发规则）
func (s *FirewallService) LiveRules(chain, family, table string) (string, error) {
	backend := s.getFirewallBackend()
	if backend != "android-iptables" && backend != "iptables" {
		return "", fmt.Errorf("当前后端 %s 不支持实时规则查看，仅 iptables 后端支持", backend)
	}

	var args []string
	if table == "nat" {
		args = append(args, "-t", "nat")
	}
	args = append(args, "-L", "--line-numbers", "-n", "-v")
	if chain != "" {
		args = append(args, "-L", chain, "--line-numbers", "-n", "-v")
	}

	useV4 := family == "" || family == "4" || family == "ipv4"
	useV6 := family == "" || family == "6" || family == "ipv6"
	var sb strings.Builder

	if useV4 {
		sb.WriteString("===== IPv4 (iptables) =====\n")
		out, err := iptablesCmd(args...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("iptables -L 失败: %s", strings.TrimSpace(string(out)))
		}
		sb.Write(out)
	}
	if useV6 {
		sb.WriteString("===== IPv6 (ip6tables) =====\n")
		out, err := ip6tablesCmd(args...).CombinedOutput()
		if err != nil {
			if family == "6" || family == "ipv6" {
				return "", fmt.Errorf("ip6tables -L 失败: %s", strings.TrimSpace(string(out)))
			}
			sb.WriteString(fmt.Sprintf("(ip6tables 不可用: %s)\n", strings.TrimSpace(string(out))))
		} else {
			sb.Write(out)
		}
	}
	return sb.String(), nil
}

// InsertRule 插入规则到指定位置（-I）
// chain: INPUT / OUTPUT / FORWARD
// position: 插入位置，1=最前面
// spec: 完整的 iptables 规则参数，如 ["-p","tcp","--dport","80","-j","ACCEPT"]
// family: "6"/"ipv6" 使用 ip6tables，其余默认 iptables
func (s *FirewallService) InsertRule(chain string, position int, spec []string, family string) error {
	backend := s.getFirewallBackend()
	if backend != "android-iptables" && backend != "iptables" {
		return fmt.Errorf("当前后端 %s 不支持插入规则", backend)
	}
	if chain == "" {
		chain = "INPUT"
	}
	if !firewallChainRe.MatchString(chain) {
		return fmt.Errorf("链名非法")
	}
	if position < 1 {
		position = 1
	}
	if position > 99999 {
		return fmt.Errorf("插入位置过大")
	}
	for _, token := range spec {
		if token == "" || !firewallSpecRe.MatchString(token) {
			return fmt.Errorf("规则参数包含非法字符: %q", token)
		}
	}
	args := append([]string{"-I", chain, fmt.Sprintf("%d", position)}, spec...)
	useV6 := family == "6" || family == "ipv6"
	name := "iptables"
	var out []byte
	var err error
	if useV6 {
		name = "ip6tables"
		out, err = ip6tablesCmd(args...).CombinedOutput()
	} else {
		out, err = iptablesCmd(args...).CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("%s -I 失败: %s", name, strings.TrimSpace(string(out)))
	}
	global.LOG.Infof("[Firewall] 插入规则: %s %d %v (family=%s)", chain, position, spec, family)
	return nil
}

// DeleteLiveRule 按行号删除系统 iptables/ip6tables 规则（-D）
// chain: INPUT / OUTPUT / FORWARD
// num: 行号（从 LiveRules 的 --line-numbers 获取）
// family: "6"/"ipv6" 操作 ip6tables，其余默认 iptables
func (s *FirewallService) DeleteLiveRule(chain string, num int, family string) error {
	backend := s.getFirewallBackend()
	if backend != "android-iptables" && backend != "iptables" {
		return fmt.Errorf("当前后端 %s 不支持删除规则", backend)
	}
	if chain == "" {
		chain = "INPUT"
	}
	if !firewallChainRe.MatchString(chain) {
		return fmt.Errorf("链名非法")
	}
	if num < 1 {
		return fmt.Errorf("行号必须大于 0")
	}
	useV6 := family == "6" || family == "ipv6"
	name := "iptables"
	var out []byte
	var err error
	if useV6 {
		name = "ip6tables"
		out, err = ip6tablesCmd("-D", chain, fmt.Sprintf("%d", num)).CombinedOutput()
	} else {
		out, err = iptablesCmd("-D", chain, fmt.Sprintf("%d", num)).CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("%s -D 失败: %s", name, strings.TrimSpace(string(out)))
	}
	global.LOG.Infof("[Firewall] 删除系统规则: %s 行号 %d (family=%s)", chain, num, family)
	return nil
}

// Lockdown 一键内网-only 模式：只允许内网+已建立连接+回环，拒绝外网（IPv4 + IPv6）
func (s *FirewallService) Lockdown() (string, error) {
	backend := s.getFirewallBackend()
	if backend != "android-iptables" && backend != "iptables" {
		return "", fmt.Errorf("当前后端 %s 不支持 Lockdown", backend)
	}

	var output []string

	// 1. 清空 INPUT 链（IPv4 + IPv6）
	iptablesCmd("-F", "INPUT").Run()
	ip6tablesCmd("-F", "INPUT").Run()
	output = append(output, "✅ 已清空 INPUT 链 (IPv4+IPv6)")

	// 2. 放行已建立连接（防止自己被踢掉）
	if err := iptablesCmd("-A", "INPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT").Run(); err == nil {
		output = append(output, "✅ 放行 ESTABLISHED,RELATED")
	}
	ip6tablesCmd("-A", "INPUT", "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT").Run()

	// 3. 放行回环
	iptablesCmd("-A", "INPUT", "-i", "lo", "-j", "ACCEPT").Run()
	ip6tablesCmd("-A", "INPUT", "-i", "lo", "-j", "ACCEPT").Run()
	output = append(output, "✅ 放行 lo 回环 (IPv4+IPv6)")

	// 4. 放行内网段（IPv4 RFC1918 + IPv6 链路本地）
	innerNets := []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"}
	for _, net := range innerNets {
		iptablesCmd("-A", "INPUT", "-s", net, "-j", "ACCEPT").Run()
	}
	ip6tablesCmd("-A", "INPUT", "-s", "fe80::/10", "-j", "ACCEPT").Run()
	output = append(output, fmt.Sprintf("✅ 放行内网段: %s + fe80::/10", strings.Join(innerNets, ", ")))

	// 5. DROP 其余全部（IPv4 + IPv6）
	iptablesCmd("-A", "INPUT", "-j", "DROP").Run()
	ip6tablesCmd("-A", "INPUT", "-j", "DROP").Run()
	output = append(output, "✅ 已设置默认 DROP (IPv4+IPv6)")

	global.LOG.Infof("[Firewall] Lockdown 模式已启用 (IPv4+IPv6)")
	return strings.Join(output, "\n"), nil
}

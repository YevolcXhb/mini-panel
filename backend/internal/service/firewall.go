package service

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
	return s.repo.Create(item)
}

func (s *FirewallService) List() ([]model.FirewallRule, error) {
	return s.repo.List()
}

func (s *FirewallService) Update(item *model.FirewallRule) error {
	return s.repo.Update(item)
}

func (s *FirewallService) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *FirewallService) getAvailableBackends() []string {
	var backends []string

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
	case "iptables":
		out, err := exec.Command("iptables", "-L", "-n").CombinedOutput()
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
		if _, err := exec.Command("iptables", "-L", "-n").CombinedOutput(); err != nil {
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
	case "iptables":
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

	// 后端工具检测
	backends := s.getAvailableBackends()
	report["available_backends"] = backends
	report["tools_installed"] = map[string]bool{
		"nft":          syscmd.Which("nft"),
		"iptables":     syscmd.Which("iptables"),
		"firewall-cmd": syscmd.Which("firewall-cmd"),
		"ufw":          syscmd.Which("ufw"),
	}

	// 内核模块检测
	kernelModules := map[string]string{}
	if syscmd.Which("iptables") {
		if out, err := exec.Command("iptables", "-L", "-n").CombinedOutput(); err != nil {
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

	// 总结
	if backends[0] == "none" {
		report["summary"] = "无法使用防火墙功能"
		report["recommendation"] = s.diagnoseNoBackend()
	} else if isRoot, ok := report["is_root"].(bool); !ok || !isRoot {
		report["summary"] = "防火墙工具已安装，但 MiniPanel 未以 root 运行，操作将被拒绝"
		report["recommendation"] = "请以 root 用户或 sudo 启动 MiniPanel"
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
		case "iptables":
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
	case "iptables":
		out, err := exec.Command("iptables", "-L", "-n").CombinedOutput()
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
		case "iptables":
			chains := []string{"INPUT", "OUTPUT", "FORWARD"}
			for _, chain := range chains {
				exec.Command("iptables", "-F", chain).Run()
			}
			exec.Command("iptables", "-P", "INPUT", "ACCEPT").Run()
			exec.Command("iptables", "-P", "OUTPUT", "ACCEPT").Run()
			exec.Command("iptables", "-P", "FORWARD", "ACCEPT").Run()
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
	case "iptables":
		chains := []string{"INPUT", "OUTPUT", "FORWARD"}
		for _, chain := range chains {
			exec.Command("iptables", "-F", chain).Run()
		}
		exec.Command("iptables", "-P", "INPUT", "ACCEPT").Run()
		exec.Command("iptables", "-P", "OUTPUT", "ACCEPT").Run()
		exec.Command("iptables", "-P", "FORWARD", "ACCEPT").Run()
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
		case "iptables":
			err = s.applyIptablesRule(&rule)
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

	if len(output) == 0 {
		return "No rules to apply", nil
	}
	return strings.Join(output, "\n"), nil
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
	case "iptables":
		exec.Command("iptables", "-A", "INPUT", "-i", "lo", "-j", "ACCEPT").Run()
		exec.Command("iptables", "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT").Run()
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
			// 检查是否是已存在规则（简单判断，忽略错误继续添加）
			checkCmd := exec.Command("iptables", "-C", chain, "-p", proto, "--dport", port, "-j", action)
			if err := checkCmd.Run(); err != nil {
				addCmd := exec.Command("iptables", "-A", chain, "-p", proto, "--dport", port, "-j", action)
				if out, err := addCmd.CombinedOutput(); err != nil {
					return fmt.Errorf("端口 %s 添加失败: %s", port, strings.TrimSpace(string(out)))
				}
			}
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		checkCmd := exec.Command("iptables", "-C", chain, "-s", rule.IP, "-j", action)
		if err := checkCmd.Run(); err != nil {
			addCmd := exec.Command("iptables", "-A", chain, "-s", rule.IP, "-j", action)
			if out, err := addCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s", strings.TrimSpace(string(out)))
			}
		}
	}
	return nil
}

func init() {
	os.MkdirAll("/etc/nftables", 0755)
}

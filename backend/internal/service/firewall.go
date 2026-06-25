package service

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

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
	// nftables直接操作内核规则集，在精简内核/嵌入式环境兼容性最好
	if syscmd.Which("nft") {
		// 测试nft是否能正常工作
		if err := exec.Command("nft", "list", "ruleset").Run(); err == nil {
			backends = append(backends, "nftables")
		}
	}

	// iptables 直接操作，比ufw依赖少
	if syscmd.Which("iptables") {
		// 测试iptables是否能正常工作
		if err := exec.Command("iptables", "-L", "-n").Run(); err == nil {
			backends = append(backends, "iptables")
		}
	}

	// firewalld 如果正在运行则使用
	if syscmd.Which("firewall-cmd") {
		out, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			backends = append(backends, "firewalld")
		}
	}

	// ufw放最后，依赖最多，在精简内核下经常无法使用
	if syscmd.Which("ufw") {
		// 不预先测试ufw，因为ufw status在未启用时也能返回结果，实际enable才会失败
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
	case "nftables":
		out, err := exec.Command("nft", "list", "ruleset").CombinedOutput()
		if err == nil {
			outStr := strings.TrimSpace(string(out))
			result["running"] = len(outStr) > 0
			// nftables只要有基础表就认为是启用状态
			if strings.Contains(outStr, "table inet filter") {
				result["running"] = true
			}
		}
	case "iptables":
		out, err := exec.Command("iptables", "-L", "-n").CombinedOutput()
		result["running"] = err == nil && len(out) > 0
	default:
		if runtime.GOOS != "linux" {
			result["message"] = "防火墙管理仅支持Linux系统"
		} else {
			result["message"] = "未检测到可用的防火墙后端 (nftables/iptables/firewalld/ufw)"
		}
	}
	return result, nil
}

func (s *FirewallService) Start() error {
	backends := s.getAvailableBackends()
	if backends[0] == "none" {
		return fmt.Errorf("未检测到可用的防火墙后端 (nftables/iptables/firewalld/ufw)")
	}

	var lastErr error
	for _, backend := range backends {
		global.LOG.Infof("[Firewall] trying to start with backend: %s", backend)
		var err error
		switch backend {
		case "firewalld":
			if out, err := exec.Command("systemctl", "start", "firewalld").CombinedOutput(); err != nil {
				if out2, err2 := exec.Command("service", "firewalld", "start").CombinedOutput(); err2 != nil {
					err = fmt.Errorf("%s | %s", strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
				} else {
					err = nil
				}
			}
		case "ufw":
			cmd := exec.Command("ufw", "--force", "enable")
			if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
				outStr := strings.TrimSpace(string(out))
				// 如果是内核模块缺失，直接跳过这个后端
				if strings.Contains(outStr, "missing kernel module") ||
					strings.Contains(outStr, "Could not fetch rule set") ||
					strings.Contains(outStr, "Problem running") {
					lastErr = fmt.Errorf("ufw: %s", outStr)
					global.LOG.Warnf("[Firewall] ufw not usable, trying next backend: %v", lastErr)
					continue
				}
				err = fmt.Errorf("ufw: %s", outStr)
			}
		case "nftables", "iptables":
			_, err = s.ApplyRulesForBackend(backend)
		}

		if err == nil {
			global.LOG.Infof("[Firewall] successfully started with backend: %s", backend)
			return nil
		}
		lastErr = err
		global.LOG.Warnf("[Firewall] backend %s failed: %v, trying next", backend, err)
	}

	return fmt.Errorf("所有防火墙后端启动均失败: %v", lastErr)
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

package service

import (
	"fmt"
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

func (s *FirewallService) getFirewallBackend() string {
	if syscmd.Which("firewall-cmd") {
		return "firewalld"
	}
	if syscmd.Which("ufw") {
		return "ufw"
	}
	if syscmd.Which("iptables") {
		return "iptables"
	}
	return "none"
}

func (s *FirewallService) GetStatus() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"backend": s.getFirewallBackend(),
		"running": false,
	}
	switch result["backend"] {
	case "firewalld":
		out, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			result["running"] = true
		}
	case "ufw":
		out, err := exec.Command("ufw", "status").CombinedOutput()
		if err == nil && strings.Contains(string(out), "Status: active") {
			result["running"] = true
		}
	case "iptables":
		out, err := exec.Command("iptables", "-L", "-n").CombinedOutput()
		result["running"] = err == nil && len(out) > 0
	default:
		if runtime.GOOS != "linux" {
			result["message"] = "防火墙管理仅支持Linux系统"
		} else {
			result["message"] = "未检测到firewalld/ufw/iptables，请先安装"
		}
	}
	return result, nil
}

func (s *FirewallService) Start() error {
	backend := s.getFirewallBackend()
	switch backend {
	case "firewalld":
		return exec.Command("systemctl", "start", "firewalld").Run()
	case "ufw":
		return exec.Command("ufw", "enable").Run()
	default:
		return fmt.Errorf("no supported firewall backend found")
	}
}

func (s *FirewallService) Stop() error {
	backend := s.getFirewallBackend()
	switch backend {
	case "firewalld":
		return exec.Command("systemctl", "stop", "firewalld").Run()
	case "ufw":
		return exec.Command("ufw", "disable").Run()
	default:
		return fmt.Errorf("no supported firewall backend found")
	}
}

func (s *FirewallService) ApplyRules() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("firewall management is only supported on Linux")
	}

	backend := s.getFirewallBackend()
	rules, err := s.repo.List()
	if err != nil {
		return "", err
	}

	var output []string
	global.LOG.Infof("[Firewall] applying %d rules using %s backend", len(rules), backend)

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
		case "iptables":
			err = s.applyIptablesRule(&rule)
		default:
			err = fmt.Errorf("no supported firewall backend found")
		}

		if err != nil {
			output = append(output, fmt.Sprintf("rule %d (%s) failed: %v", rule.ID, rule.Name, err))
		} else {
			output = append(output, fmt.Sprintf("rule %d (%s) applied", rule.ID, rule.Name))
		}
	}

	if backend == "firewalld" {
		exec.Command("firewall-cmd", "--runtime-to-permanent").Run()
	}

	if len(output) == 0 {
		return "No rules to apply", nil
	}
	return strings.Join(output, "\n"), nil
}

func (s *FirewallService) applyFirewalldRule(rule *model.FirewallRule) error {
	if rule.Type == "port" && rule.Port != "" {
		action := "--add-port"
		if rule.Action == "deny" {
			action = "--remove-port"
		}
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		portSpec := fmt.Sprintf("%s/%s", rule.Port, proto)
		cmd := exec.Command("firewall-cmd", action, portSpec, "--permanent")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", err, string(out))
		}
		if rule.Direction == "in" {
			zoneCmd := exec.Command("firewall-cmd", "--add-service", "http", "--permanent")
			zoneCmd.Run()
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		richRule := ""
		if rule.Action == "allow" {
			richRule = fmt.Sprintf("rule family='ipv4' source address='%s' accept", rule.IP)
		} else {
			richRule = fmt.Sprintf("rule family='ipv4' source address='%s' drop", rule.IP)
		}
		cmd := exec.Command("firewall-cmd", "--add-rich-rule", richRule, "--permanent")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", err, string(out))
		}
	}
	return exec.Command("firewall-cmd", "--reload").Run()
}

func (s *FirewallService) applyUfwRule(rule *model.FirewallRule) error {
	if rule.Type == "port" && rule.Port != "" {
		action := "allow"
		if rule.Action == "deny" {
			action = "deny"
		}
		proto := rule.Protocol
		if proto == "" || proto == "all" {
			proto = "tcp"
		}
		cmd := exec.Command("ufw", action, proto, rule.Port)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", err, string(out))
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		action := "allow"
		if rule.Action == "deny" {
			action = "deny"
		}
		cmd := exec.Command("ufw", action, "from", rule.IP)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %s", err, string(out))
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
		checkCmd := exec.Command("iptables", "-C", chain, "-p", proto, "--dport", rule.Port, "-j", action)
		if err := checkCmd.Run(); err != nil {
			addCmd := exec.Command("iptables", "-A", chain, "-p", proto, "--dport", rule.Port, "-j", action)
			if out, err := addCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %s", err, string(out))
			}
		}
	} else if rule.Type == "ip" && rule.IP != "" {
		checkCmd := exec.Command("iptables", "-C", chain, "-s", rule.IP, "-j", action)
		if err := checkCmd.Run(); err != nil {
			addCmd := exec.Command("iptables", "-A", chain, "-s", rule.IP, "-j", action)
			if out, err := addCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %s", err, string(out))
			}
		}
	}
	return nil
}

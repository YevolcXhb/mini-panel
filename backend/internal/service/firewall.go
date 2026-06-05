package service

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/model"
	"github.com/minipanel/minipanel/internal/repository"
)

type FirewallService struct {
	repo *repository.FirewallRepository
}

func NewFirewallService() *FirewallService {
	return &FirewallService{repo: repository.NewFirewallRepository(global.DB)}
}

func (s *FirewallService) Create(item *model.FirewallRule) error {
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

func (s *FirewallService) ApplyRules() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("firewall management is only supported on Linux")
	}

	rules, err := s.repo.List()
	if err != nil {
		return "", err
	}

	var output []string
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Type == "port" && rule.Port != "" {
			cmd := exec.Command("iptables", "-C", "INPUT", "-p", rule.Protocol, "--dport", rule.Port, "-j", strings.ToUpper(rule.Action))
			if err := cmd.Run(); err != nil {
				cmd = exec.Command("iptables", "-A", "INPUT", "-p", rule.Protocol, "--dport", rule.Port, "-j", strings.ToUpper(rule.Action))
				if out, err := cmd.CombinedOutput(); err != nil {
					output = append(output, fmt.Sprintf("failed to apply rule %d: %v (%s)", rule.ID, err, string(out)))
				} else {
					output = append(output, fmt.Sprintf("applied rule %d: %s port %s/%s", rule.ID, rule.Action, rule.Port, rule.Protocol))
				}
			}
		}
	}

	if len(output) == 0 {
		return "No new rules to apply", nil
	}
	return strings.Join(output, "\n"), nil
}

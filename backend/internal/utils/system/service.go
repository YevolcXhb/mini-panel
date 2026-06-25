package system

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/minipanel/minipanel/internal/utils/cmd"
)

type ServiceStatus struct {
	Name        string `json:"name"`
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	Version     string `json:"version,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	Message     string `json:"message,omitempty"`
}

func CheckNginx() ServiceStatus {
	status := ServiceStatus{Name: "nginx", ServiceName: "nginx"}

	if !cmd.Which("nginx") {
		status.Installed = false
		status.Message = "Nginx未安装"
		return status
	}
	status.Installed = true

	out, err := exec.Command("nginx", "-v").CombinedOutput()
	if err == nil {
		versionStr := strings.TrimSpace(string(out))
		parts := strings.Split(versionStr, "/")
		if len(parts) >= 2 {
			status.Version = strings.Split(parts[1], " ")[0]
		}
	}

	if runtime.GOOS == "linux" {
		for _, svc := range []string{"nginx", "nginx.service"} {
			out, err := exec.Command("systemctl", "is-active", svc).CombinedOutput()
			if err == nil && strings.TrimSpace(string(out)) == "active" {
				status.Running = true
				return status
			}
		}
		out, err := exec.Command("pgrep", "nginx").CombinedOutput()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			status.Running = true
		}
	}
	return status
}

func CheckMySQL() ServiceStatus {
	status := ServiceStatus{Name: "mysql", ServiceName: "mysqld"}

	if !cmd.Which("mysqld") && !cmd.Which("mariadbd") {
		if !cmd.Which("mysql") {
			status.Installed = false
			status.Message = "MySQL/MariaDB未安装"
			return status
		}
	}
	status.Installed = true

	if cmd.Which("mysql") {
		out, err := exec.Command("mysql", "--version").CombinedOutput()
		if err == nil {
			versionStr := strings.TrimSpace(string(out))
			parts := strings.Fields(versionStr)
			for i, p := range parts {
				if strings.Contains(p, "Distrib") && i > 0 {
					status.Version = strings.TrimRight(parts[i+1], ",")
					break
				}
			}
		}
	}

	if runtime.GOOS == "linux" {
		for _, svc := range []string{"mysql", "mysqld", "mariadb", "mariadb.service"} {
			out, err := exec.Command("systemctl", "is-active", svc).CombinedOutput()
			if err == nil && strings.TrimSpace(string(out)) == "active" {
				status.Running = true
				status.ServiceName = svc
				return status
			}
		}
		out, err := exec.Command("pgrep", "-f", "mysqld|mariadbd").CombinedOutput()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			status.Running = true
		}
	}
	return status
}

func CheckFirewalld() ServiceStatus {
	status := ServiceStatus{Name: "firewalld", ServiceName: "firewalld"}

	if !cmd.Which("firewall-cmd") && !cmd.Which("ufw") {
		status.Installed = false
		if runtime.GOOS != "linux" {
			status.Message = "防火墙管理仅支持Linux"
		} else {
			status.Message = "未检测到firewalld或ufw"
		}
		return status
	}
	status.Installed = true

	if cmd.Which("firewall-cmd") {
		out, err := exec.Command("firewall-cmd", "--state").CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			status.Running = true
		}
	} else if cmd.Which("ufw") {
		out, err := exec.Command("ufw", "status").CombinedOutput()
		if err == nil && strings.Contains(string(out), "Status: active") {
			status.Running = true
			status.Name = "ufw"
			status.ServiceName = "ufw"
		}
	}
	return status
}

func GetAllServices() map[string]ServiceStatus {
	return map[string]ServiceStatus{
		"nginx":     CheckNginx(),
		"mysql":     CheckMySQL(),
		"firewalld": CheckFirewalld(),
	}
}

func InstallService(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("service installation is only supported on Linux")
	}

	var installCmd *exec.Cmd
	switch name {
	case "nginx":
		if cmd.Which("apt-get") {
			installCmd = exec.Command("apt-get", "install", "-y", "nginx")
		} else if cmd.Which("yum") {
			installCmd = exec.Command("yum", "install", "-y", "nginx")
		} else if cmd.Which("dnf") {
			installCmd = exec.Command("dnf", "install", "-y", "nginx")
		}
	case "mysql", "mariadb":
		if cmd.Which("apt-get") {
			installCmd = exec.Command("apt-get", "install", "-y", "mariadb-server")
		} else if cmd.Which("yum") {
			installCmd = exec.Command("yum", "install", "-y", "mariadb-server")
		} else if cmd.Which("dnf") {
			installCmd = exec.Command("dnf", "install", "-y", "mariadb-server")
		}
	default:
		return fmt.Errorf("unsupported service: %s", name)
	}

	if installCmd == nil {
		return fmt.Errorf("unsupported package manager, please install %s manually", name)
	}

	installCmd.Stdout = nil
	installCmd.Stderr = nil
	return installCmd.Run()
}

func StartService(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("service management is only supported on Linux")
	}

	for _, svc := range getServiceNames(name) {
		cmd := exec.Command("systemctl", "start", svc)
		if err := cmd.Run(); err == nil {
			exec.Command("systemctl", "enable", svc).Run()
			return nil
		}
	}
	return fmt.Errorf("failed to start service %s", name)
}

func StopService(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("service management is only supported on Linux")
	}

	for _, svc := range getServiceNames(name) {
		exec.Command("systemctl", "stop", svc).Run()
	}
	return nil
}

func RestartService(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("service management is only supported on Linux")
	}

	for _, svc := range getServiceNames(name) {
		cmd := exec.Command("systemctl", "restart", svc)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to restart service %s", name)
}

func getServiceNames(name string) []string {
	switch name {
	case "nginx":
		return []string{"nginx", "nginx.service"}
	case "mysql", "mariadb":
		return []string{"mysql", "mysqld", "mariadb", "mariadb.service"}
	case "firewalld":
		return []string{"firewalld", "firewalld.service"}
	case "ufw":
		return []string{"ufw"}
	default:
		return []string{name}
	}
}

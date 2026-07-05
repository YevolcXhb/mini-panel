package system

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

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
			// 检测是否是MariaDB
			if strings.Contains(strings.ToLower(string(out)), "mariadb") || cmd.Which("mariadbd") {
				status.Name = "mariadb"
			}
		}
	}

	if runtime.GOOS == "linux" {
		running := false
		// 先尝试systemctl
		if cmd.Which("systemctl") {
			for _, svc := range []string{"mysql", "mysqld", "mariadb", "mariadb.service"} {
				out, err := exec.Command("systemctl", "is-active", svc).CombinedOutput()
				if err == nil && strings.TrimSpace(string(out)) == "active" {
					running = true
					status.ServiceName = svc
					break
				}
			}
		}
		// 进程检测
		if !running {
			out, err := exec.Command("pgrep", "-f", "mysqld|mariadbd|mariadbd-safe|mysqld_safe").CombinedOutput()
			if err == nil && len(strings.TrimSpace(string(out))) > 0 {
				running = true
			}
		}
		// 端口检测
		if !running {
			conn, err := net.DialTimeout("tcp", "127.0.0.1:3306", 2*time.Second)
			if err == nil {
				conn.Close()
				running = true
			}
		}
		status.Running = running
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

	var lastErr error
	// 先尝试systemctl
	if cmd.Which("systemctl") {
		for _, svc := range getServiceNames(name) {
			cmd := exec.Command("systemctl", "start", svc)
			if out, err := cmd.CombinedOutput(); err == nil {
				exec.Command("systemctl", "enable", svc).Run()
				// 等待一下再验证
				time.Sleep(500 * time.Millisecond)
				if checkServiceRunning(name) {
					return nil
				}
				// systemctl返回成功但进程没起来，继续尝试其他方法
			} else {
				lastErr = fmt.Errorf("%s: %s", svc, strings.TrimSpace(string(out)))
			}
		}
	}

	// systemctl失败或不存在，尝试service命令
	if cmd.Which("service") {
		for _, svc := range getServiceNames(name) {
			cmd := exec.Command("service", svc, "start")
			if out, err := cmd.CombinedOutput(); err == nil {
				time.Sleep(500 * time.Millisecond)
				if checkServiceRunning(name) {
					return nil
				}
			} else {
				errStr := strings.TrimSpace(string(out))
				if errStr != "" {
					lastErr = fmt.Errorf("service %s: %s", svc, errStr)
				}
			}
		}
	}

	// 最后尝试直接启动
	if err := directStartService(name); err == nil {
		time.Sleep(1 * time.Second)
		if checkServiceRunning(name) {
			return nil
		}
	} else if lastErr == nil {
		lastErr = err
	}

	if lastErr != nil {
		return fmt.Errorf("启动服务失败: %v (请尝试手动启动)", lastErr)
	}
	if !checkServiceRunning(name) {
		return fmt.Errorf("启动服务后未检测到运行进程，请检查服务配置和错误日志")
	}
	return nil
}

func checkServiceRunning(name string) bool {
	switch name {
	case "nginx":
		return CheckNginx().Running
	case "mysql", "mariadb":
		return CheckMySQL().Running
	case "firewalld", "ufw":
		return CheckFirewalld().Running
	default:
		out, err := exec.Command("pgrep", name).CombinedOutput()
		return err == nil && len(strings.TrimSpace(string(out))) > 0
	}
}

func directStartService(name string) error {
	switch name {
	case "mysql", "mariadb":
		// 尝试常见启动方式
		for _, startCmd := range []string{"mariadbd-safe", "mysqld_safe", "mysqld", "mariadbd"} {
			if cmd.Which(startCmd) {
				var cmd *exec.Cmd
				if strings.Contains(startCmd, "safe") {
					cmd = exec.Command(startCmd, "--user", "mysql", "--datadir", "/var/lib/mysql")
				} else {
					cmd = exec.Command(startCmd, "--user", "mysql", "--datadir", "/var/lib/mysql", "--daemonize")
				}
				cmd.Start()
				return nil
			}
		}
		return fmt.Errorf("could not find mysql/mariadb daemon binary")
	case "nginx":
		if cmd.Which("nginx") {
			cmd := exec.Command("nginx")
			return cmd.Start()
		}
	}
	return fmt.Errorf("direct start not supported for %s", name)
}

func StopService(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("service management is only supported on Linux")
	}

	// 先尝试systemctl
	if cmd.Which("systemctl") {
		for _, svc := range getServiceNames(name) {
			exec.Command("systemctl", "stop", svc).Run()
			time.Sleep(500 * time.Millisecond)
			if !checkServiceRunning(name) {
				return nil
			}
		}
	}

	// 尝试service命令
	if cmd.Which("service") {
		for _, svc := range getServiceNames(name) {
			exec.Command("service", svc, "stop").Run()
			time.Sleep(500 * time.Millisecond)
			if !checkServiceRunning(name) {
				return nil
			}
		}
	}

	// 直接杀死进程
	directStopService(name)
	time.Sleep(1 * time.Second)
	if !checkServiceRunning(name) {
		return nil
	}
	return fmt.Errorf("停止服务失败: 进程 %s 仍在运行，请检查服务状态", name)
}

func directStopService(name string) {
	switch name {
	case "mysql", "mariadb":
		exec.Command("pkill", "-f", "mysqld|mariadbd").Run()
	case "nginx":
		exec.Command("pkill", "nginx").Run()
	}
}

func RestartService(name string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("service management is only supported on Linux")
	}

	var lastErr error
	// 先尝试systemctl
	if cmd.Which("systemctl") {
		for _, svc := range getServiceNames(name) {
			if out, err := exec.Command("systemctl", "restart", svc).CombinedOutput(); err == nil {
				time.Sleep(500 * time.Millisecond)
				if checkServiceRunning(name) {
					return nil
				}
			} else {
				lastErr = fmt.Errorf("%s: %s", svc, strings.TrimSpace(string(out)))
			}
		}
	}

	// systemctl失败尝试service命令
	if cmd.Which("service") {
		for _, svc := range getServiceNames(name) {
			if out, err := exec.Command("service", svc, "restart").CombinedOutput(); err == nil {
				time.Sleep(500 * time.Millisecond)
				if checkServiceRunning(name) {
					return nil
				}
			} else {
				errStr := strings.TrimSpace(string(out))
				if errStr != "" {
					lastErr = fmt.Errorf("service %s: %s", svc, errStr)
				}
			}
		}
	}

	// 直接重启：先停止再启动
	StopService(name)
	time.Sleep(1 * time.Second)
	if err := directStartService(name); err == nil {
		time.Sleep(1 * time.Second)
		if checkServiceRunning(name) {
			return nil
		}
	}

	if !checkServiceRunning(name) {
		if lastErr != nil {
			return fmt.Errorf("重启服务失败: %v", lastErr)
		}
		return fmt.Errorf("重启服务后未检测到运行进程")
	}
	return nil
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

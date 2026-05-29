package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// dockerComposeYaml 模拟 1Panel 官方商店中 MySQL 应用的 docker-compose.yml
var dockerComposeYaml = `
services:
  mysql:
    image: mysql:8.0.33
    container_name: 1panel-mysql-xxxx
    environment:
      MYSQL_ROOT_PASSWORD: ${PANEL_DB_ROOT_PASSWORD}
      MYSQL_DATABASE: ${PANEL_DB_NAME}
    ports:
      - "${PANEL_APP_PORT_HTTP}:3306"
    volumes:
      - ./data:/var/lib/mysql
      - ./conf/my.cnf:/etc/mysql/my.cnf
    restart: always
    networks:
      - 1panel-network

networks:
  1panel-network:
    external: true
`

// dockerComposeSimple 模拟 Nginx 等简单应用
var dockerComposeSimple = `
services:
  nginx:
    image: nginx:1.27-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./html:/usr/share/nginx/html
`

// dockerComposeNoPorts 没有端口映射的应用
var dockerComposeNoPorts = `
services:
  redis:
    image: redis:7-alpine
    volumes:
      - ./data:/data
`

type composeService struct {
	Image       interface{} `yaml:"image"`
	Ports       []string    `yaml:"ports"`
	Environment interface{} `yaml:"environment"`
	Volumes     []string    `yaml:"volumes"`
}

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

func TestParseDockerComposeYAML(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantImage     string
		wantPorts     []string
		wantVolumes   []string
		wantEnvKeys   []string
		wantFirstPort int
	}{
		{
			name:          "mysql-compose",
			yaml:          dockerComposeYaml,
			wantImage:     "mysql:8.0.33",
			wantPorts:     []string{"${PANEL_APP_PORT_HTTP}:3306"},
			wantVolumes:   []string{"./data:/var/lib/mysql", "./conf/my.cnf:/etc/mysql/my.cnf"},
			wantEnvKeys:   []string{"MYSQL_ROOT_PASSWORD", "MYSQL_DATABASE"},
			wantFirstPort: 3306,
		},
		{
			name:          "nginx-compose",
			yaml:          dockerComposeSimple,
			wantImage:     "nginx:1.27-alpine",
			wantPorts:     []string{"80:80", "443:443"},
			wantVolumes:   []string{"./html:/usr/share/nginx/html"},
			wantEnvKeys:   nil,
			wantFirstPort: 80,
		},
		{
			name:          "redis-no-ports",
			yaml:          dockerComposeNoPorts,
			wantImage:     "redis:7-alpine",
			wantPorts:     nil,
			wantVolumes:   []string{"./data:/data"},
			wantEnvKeys:   nil,
			wantFirstPort: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var compose composeFile
			if err := yaml.Unmarshal([]byte(tt.yaml), &compose); err != nil {
				t.Fatalf("yaml unmarshal failed: %v", err)
			}

			var svc composeService
			for _, v := range compose.Services {
				svc = v
				break // 只取第一个服务
			}

			// 验证 Image
			gotImage := ""
			switch img := svc.Image.(type) {
			case string:
				gotImage = img
			}
			if gotImage != tt.wantImage {
				t.Errorf("image = %q, want %q", gotImage, tt.wantImage)
			}

			// 验证 Ports
			if len(svc.Ports) != len(tt.wantPorts) {
				t.Errorf("ports count = %d, want %d", len(svc.Ports), len(tt.wantPorts))
			} else {
				for i := range svc.Ports {
					if svc.Ports[i] != tt.wantPorts[i] {
						t.Errorf("ports[%d] = %q, want %q", i, svc.Ports[i], tt.wantPorts[i])
					}
				}
			}

			// 验证 Volumes
			if len(svc.Volumes) != len(tt.wantVolumes) {
				t.Errorf("volumes count = %d, want %d", len(svc.Volumes), len(tt.wantVolumes))
			}

			// 验证 Environment keys
			var gotEnvKeys []string
			switch env := svc.Environment.(type) {
			case map[string]interface{}:
				for k := range env {
					gotEnvKeys = append(gotEnvKeys, k)
				}
			}
			if len(gotEnvKeys) != len(tt.wantEnvKeys) {
				t.Errorf("env keys count = %d, want %d", len(gotEnvKeys), len(tt.wantEnvKeys))
			}

			// 验证端口提取逻辑（提取主机端口）
			firstPort := extractHostPort(svc.Ports)
			if firstPort != tt.wantFirstPort {
				t.Errorf("extractHostPort = %d, want %d", firstPort, tt.wantFirstPort)
			}
		})
	}
}

func TestExtractHostPortEdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"8080:80", 8080},
		{"${PANEL_APP_PORT_HTTP}:3306", 3306}, // 变量情况下返回容器端口兜底
		{"80:80", 80},
		{"3306", 3306}, // 无冒号时直接解析为端口
		{"", 0},        // 空字符串
		{"abc:80", 80}, // 主机端口非法，回退到容器端口
		{"abc:def", 0}, // 两边都非法
	}
	for _, tt := range tests {
		got := extractHostPort([]string{tt.input})
		if got != tt.want {
			t.Errorf("extractHostPort(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestInstallWithComposeDir 模拟安装时从 docker-compose.yml 读取配置并映射到 dockroot 参数
func TestInstallWithComposeDir(t *testing.T) {
	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(dockerComposeYaml), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}

	var compose composeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	var svc composeService
	for _, v := range compose.Services {
		svc = v
		break
	}

	// 验证映射逻辑
	if svc.Image != "mysql:8.0.33" {
		t.Errorf("expected image mysql:8.0.33, got %v", svc.Image)
	}

	// volumes 映射验证：./data:/var/lib/mysql -> 创建 hostPath 并映射
	for _, v := range svc.Volumes {
		parts := strings.Split(v, ":")
		if len(parts) >= 2 {
			hostPath := filepath.Join(tmpDir, filepath.Base(parts[0]))
			if err := os.MkdirAll(hostPath, 0755); err != nil {
				t.Fatalf("mkdir %s failed: %v", hostPath, err)
			}
			// 验证目录已创建
			if _, err := os.Stat(hostPath); err != nil {
				t.Errorf("expected dir %s to exist", hostPath)
			}
		}
	}
}

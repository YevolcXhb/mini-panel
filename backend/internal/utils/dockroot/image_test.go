package dockroot

import (
	"reflect"
	"testing"
)

func TestNormalizeImageRef(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"nginx", "nginx:latest"},
		{"nginx:latest", "nginx:latest"},
		{"nginx:alpine", "nginx:alpine"},
		{"registry.linkease.net:5443/nginx:latest", "registry.linkease.net:5443/nginx:latest"},
		{"docker.1ms.run/openlistteam/openlist:latest-lite-aio", "docker.1ms.run/openlistteam/openlist:latest-lite-aio"},
		{"docker://nginx:latest", "docker://nginx:latest"},
		{"docker://registry.host:5000/ns/name:1.0", "docker://registry.host:5000/ns/name:1.0"},
		{"redis@sha256:abc123", "redis:latest"},
		{"", ""},
	}

	for _, tt := range tests {
		got := NormalizeImageRef(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeImageRef(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestExtractContainerName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"nginx:latest", "nginx"},
		{"openlist:latest-lite-aio", "openlist"},
		{"docker.1ms.run/openlistteam/openlist:latest", "openlist"},
		{"registry.linkease.net:5443/nginx:alpine", "nginx"},
		{"docker.io/library/redis:latest", "redis"},
		{"docker://docker.1ms.run/ns/my-app:1.0", "my-app"},
		{"redis@sha256:abc123", "redis"},
		{"", ""},
	}

	for _, tt := range tests {
		got := ExtractContainerName(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractContainerName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestPullCandidates(t *testing.T) {
	c := &Client{
		Mirrors: []string{
			"https://docker.1ms.run",
			"https://docker.m.daocloud.io/",
			"http://kooldocker.openpop.cn",
			"",
			"   ",
		},
	}
	tests := []struct {
		name  string
		image string
		want  []string
	}{
		{
			name:  "bare image uses mirrors then docker hub",
			image: "nginx:latest",
			want: []string{
				"docker://docker.1ms.run/nginx:latest",
				"docker://docker.m.daocloud.io/nginx:latest",
				"docker://kooldocker.openpop.cn/nginx:latest",
				"docker://docker.io/nginx:latest",
			},
		},
		{
			name:  "docker prefix bare image is treated as bare",
			image: "docker://nginx:latest",
			want: []string{
				"docker://docker.1ms.run/nginx:latest",
				"docker://docker.m.daocloud.io/nginx:latest",
				"docker://kooldocker.openpop.cn/nginx:latest",
				"docker://docker.io/nginx:latest",
			},
		},
		{
			name:  "explicit mirror registry is used as is",
			image: "docker.1ms.run/openlistteam/openlist:latest",
			want:  []string{"docker.1ms.run/openlistteam/openlist:latest"},
		},
		{
			name:  "explicit registry with port is used as is",
			image: "registry.linkease.net:5443/nginx:latest",
			want:  []string{"registry.linkease.net:5443/nginx:latest"},
		},
		{
			name:  "explicit docker transport is used as is",
			image: "docker://registry.host:5000/ns/name:1.0",
			want:  []string{"docker://registry.host:5000/ns/name:1.0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.pullCandidates(tt.image)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("pullCandidates(%q) = %v, want %v", tt.image, got, tt.want)
			}
		})
	}
}

func TestMirrorHost(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://docker.1ms.run", "docker.1ms.run"},
		{"https://docker1.linkease.com:60005", "docker1.linkease.com:60005"},
		{"http://kooldocker.openpop.cn", "kooldocker.openpop.cn"},
		{"https://docker.m.daocloud.io/", "docker.m.daocloud.io"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := mirrorHost(tt.in); got != tt.want {
			t.Errorf("mirrorHost(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

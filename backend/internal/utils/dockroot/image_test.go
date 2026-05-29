package dockroot

import "testing"

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

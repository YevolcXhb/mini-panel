package main

import (
	"reflect"
	"testing"
)

func TestBuildPullSources(t *testing.T) {
	mirrors := []string{
		"https://docker.1ms.run",
		"https://docker.m.daocloud.io/",
		"",
		"   ",
	}
	tests := []struct {
		name, ref, imageName, tag string
		want                      []string
	}{
		{
			name:      "bare image uses mirrors then docker hub",
			ref:       "nginx:latest",
			imageName: "nginx",
			tag:       "latest",
			want: []string{
				"docker://docker.1ms.run/nginx:latest",
				"docker://docker.m.daocloud.io/nginx:latest",
				"docker://docker.io/nginx:latest",
			},
		},
		{
			name:      "docker prefix bare image is treated as bare",
			ref:       "docker://nginx:latest",
			imageName: "nginx",
			tag:       "latest",
			want: []string{
				"docker://docker.1ms.run/nginx:latest",
				"docker://docker.m.daocloud.io/nginx:latest",
				"docker://docker.io/nginx:latest",
			},
		},
		{
			name:      "empty tag defaults to latest",
			ref:       "redis",
			imageName: "redis",
			tag:       "",
			want: []string{
				"docker://docker.1ms.run/redis:latest",
				"docker://docker.m.daocloud.io/redis:latest",
				"docker://docker.io/redis:latest",
			},
		},
		{
			name:      "explicit mirror registry is used as is",
			ref:       "docker.1ms.run/openlistteam/openlist:latest",
			imageName: "docker.1ms.run/openlistteam/openlist",
			tag:       "latest",
			want:      []string{"docker://docker.1ms.run/openlistteam/openlist:latest"},
		},
		{
			name:      "explicit registry with port is used as is",
			ref:       "registry.linkease.net:5443/nginx:alpine",
			imageName: "registry.linkease.net:5443/nginx",
			tag:       "alpine",
			want:      []string{"docker://registry.linkease.net:5443/nginx:alpine"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPullSources(tt.ref, tt.imageName, tt.tag, mirrors)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildPullSources(%q, %q, mirrors) = %v, want %v", tt.ref, tt.tag, got, tt.want)
			}
		})
	}
}

func TestMirrorHostDockRoot(t *testing.T) {
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

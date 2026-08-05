package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/minipanel/minipanel/internal/model"
)

func TestNginxListenPorts(t *testing.T) {
	cases := []struct {
		name      string
		website   model.Website
		wantHTTP  int
		wantHTTPS int
	}{
		{name: "ssl-standard-443", website: model.Website{SSL: true, Port: 443}, wantHTTP: 80, wantHTTPS: 443},
		{name: "ssl-default-80", website: model.Website{SSL: true, Port: 80}, wantHTTP: 80, wantHTTPS: 443},
		{name: "ssl-zero-port", website: model.Website{SSL: true}, wantHTTP: 80, wantHTTPS: 443},
		{name: "ssl-custom-port", website: model.Website{SSL: true, Port: 8443}, wantHTTP: 80, wantHTTPS: 8443},
		{name: "plain-8080", website: model.Website{Port: 8080}, wantHTTP: 8080, wantHTTPS: 0},
		{name: "plain-zero", website: model.Website{}, wantHTTP: 80, wantHTTPS: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpPort, httpsPort := nginxListenPorts(&tc.website)
			if httpPort != tc.wantHTTP || httpsPort != tc.wantHTTPS {
				t.Fatalf("nginxListenPorts(%+v) = (%d, %d), want (%d, %d)",
					tc.website, httpPort, httpsPort, tc.wantHTTP, tc.wantHTTPS)
			}
		})
	}
}

func TestNormalizeWebsitePort(t *testing.T) {
	ssl80 := &model.Website{SSL: true, Port: 80}
	if !normalizeWebsitePort(ssl80) || ssl80.Port != 443 {
		t.Fatalf("SSL port 80 should normalize to 443, got %d", ssl80.Port)
	}
	ssl443 := &model.Website{SSL: true, Port: 443}
	if normalizeWebsitePort(ssl443) || ssl443.Port != 443 {
		t.Fatalf("SSL port 443 should be kept, got %d", ssl443.Port)
	}
	plain := &model.Website{}
	if !normalizeWebsitePort(plain) || plain.Port != 80 {
		t.Fatalf("plain port 0 should normalize to 80, got %d", plain.Port)
	}
	custom := &model.Website{SSL: true, Port: 8443}
	if normalizeWebsitePort(custom) || custom.Port != 8443 {
		t.Fatalf("custom SSL port should be kept, got %d", custom.Port)
	}
}

func TestSanitizeNginxFileName(t *testing.T) {
	cases := map[string]string{
		"example.com":     "example.com",
		"*.example.com":   "_.example.com",
		"a b/c":           "a_b_c",
		"":                "default",
		"my_domain-1.com": "my_domain-1.com",
	}
	for in, want := range cases {
		if got := sanitizeNginxFileName(in); got != want {
			t.Errorf("sanitizeNginxFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSafeToOverwriteConfig(t *testing.T) {
	svc := &WebsiteService{}
	dir := t.TempDir()

	// 不存在的文件视为可覆盖
	missing := filepath.Join(dir, "missing.conf")
	if !svc.safeToOverwriteConfig(missing) {
		t.Fatal("missing file should be safe to overwrite")
	}

	// 面板生成且只包含一个站点的文件可以覆盖
	managedSingle := filepath.Join(dir, "managed-single.conf")
	if err := os.WriteFile(managedSingle, []byte("# Managed by MiniPanel\nserver {\n    server_name example.com;\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !svc.safeToOverwriteConfig(managedSingle) {
		t.Fatal("managed single-site file should be safe to overwrite")
	}

	// 手动维护的文件（无面板标记）不可覆盖
	manual := filepath.Join(dir, "manual.conf")
	if err := os.WriteFile(manual, []byte("server {\n    server_name manual.com;\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if svc.safeToOverwriteConfig(manual) {
		t.Fatal("manual file should NOT be overwritten")
	}

	// 面板生成但包含多个站点的共享文件不可覆盖
	shared := filepath.Join(dir, "shared.conf")
	content := "# Managed by MiniPanel\nserver {\n    server_name a.com;\n}\nserver {\n    server_name b.com;\n}\n"
	if err := os.WriteFile(shared, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if svc.safeToOverwriteConfig(shared) {
		t.Fatal("shared multi-site file should NOT be overwritten")
	}
}

func TestExtractSiteFromServerBlock(t *testing.T) {
	// 标准 SSL 站点：80 跳转 + 443，端口应取 443
	block := `
server {
    listen 80;
    listen 443 ssl http2;
    server_name alist.example.com;
    ssl_certificate /x/fullchain.pem;
    proxy_pass http://127.0.0.1:5212;
}`
	site := extractSiteFromServerBlock(block, "alist.conf", "default")
	if site == nil {
		t.Fatal("expected site, got nil")
	}
	if site.Port != 443 || !site.SSL || site.Domain != "alist.example.com" {
		t.Fatalf("unexpected site: %+v", site)
	}

	// 自定义 HTTPS 端口
	custom := `
server {
    listen 8443 ssl;
    server_name custom.example.com;
}`
	customSite := extractSiteFromServerBlock(custom, "custom.conf", "default")
	if customSite == nil || customSite.Port != 8443 || !customSite.SSL {
		t.Fatalf("unexpected custom ssl site: %+v", customSite)
	}

	// 纯 catch-all 不应作为站点展示
	catchAll := `
server {
    listen 80 default_server;
    server_name _;
    return 444;
}`
	if got := extractSiteFromServerBlock(catchAll, "00-minipanel-default.conf", "default"); got != nil {
		t.Fatalf("catch-all should be skipped, got %+v", got)
	}
}

func TestParseNginxConfigContent(t *testing.T) {
	svc := &WebsiteService{}

	// 面板默认 catch-all 文件整体跳过
	content := `
server {
    listen 80 default_server;
    server_name _;
    return 444;
}`
	if sites := svc.parseNginxConfigContent(content, "00-minipanel-default"); len(sites) != 0 {
		t.Fatalf("isolation file should be skipped, got %d sites", len(sites))
	}

	// 普通文件中的 catch-all 块跳过，真实站点保留
	multi := `
server {
    listen 80 default_server;
    server_name _;
    return 444;
}
server {
    listen 80;
    server_name real.example.com;
}`
	sites := svc.parseNginxConfigContent(multi, "sites.conf")
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d: %+v", len(sites), sites)
	}
	if sites[0].Domain != "real.example.com" {
		t.Fatalf("unexpected domain: %+v", sites[0])
	}
}

func TestBuildWebsiteNginxConfig(t *testing.T) {
	svc := &WebsiteService{}

	// 标准 SSL 代理站点：80 跳转、443 监听，绝不允许 886/523 等错误端口
	w := &model.Website{
		Name:              "alist",
		Domain:            "alist.example.com",
		Port:              443,
		Type:              "proxy",
		ProxyTarget:       "http://127.0.0.1:5212",
		ClientMaxBodySize: "10240M",
		SSL:               true,
		SSLCert:           "/opt/data/ssl/alist.example.com/fullchain.pem",
		SSLKey:            "/opt/data/ssl/alist.example.com/privkey.pem",
	}
	content, certPath, keyPath := svc.buildWebsiteNginxConfig(w)
	if content == "" {
		t.Fatal("generated config should not be empty")
	}
	for _, want := range []string{
		"listen 80;",
		"listen [::]:80;",
		"listen 443 ssl http2;",
		"listen [::]:443 ssl http2;",
		"return 301 https://$host$request_uri;",
		"proxy_pass http://127.0.0.1:5212;",
		"client_max_body_size 10240M;",
		"ssl_certificate /opt/data/ssl/alist.example.com/fullchain.pem;",
		"ssl_certificate_key /opt/data/ssl/alist.example.com/privkey.pem;",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("generated config missing %q:\n%s", want, content)
		}
	}
	for _, banned := range []string{"listen 886", "listen 523", "listen 923"} {
		if strings.Contains(content, banned) {
			t.Errorf("generated config contains forbidden listen port %q:\n%s", banned, content)
		}
	}
	if certPath != w.SSLCert || keyPath != w.SSLKey {
		t.Fatalf("unexpected cert paths: %q %q", certPath, keyPath)
	}

	// 端口 80 的 SSL 站点同样生成标准 80/443
	w80 := *w
	w80.Port = 80
	content80, _, _ := svc.buildWebsiteNginxConfig(&w80)
	if !strings.Contains(content80, "listen 443 ssl http2;") {
		t.Errorf("SSL site with port 80 should listen on 443:\n%s", content80)
	}
	if strings.Contains(content80, "listen 523") {
		t.Errorf("SSL site with port 80 must not listen on 523:\n%s", content80)
	}

	// 自定义 HTTPS 端口：跳转带上端口
	wCustom := *w
	wCustom.Port = 8443
	contentCustom, _, _ := svc.buildWebsiteNginxConfig(&wCustom)
	if !strings.Contains(contentCustom, "listen 8443 ssl http2;") {
		t.Errorf("custom SSL port should listen on 8443:\n%s", contentCustom)
	}
	if !strings.Contains(contentCustom, "return 301 https://$host:8443$request_uri;") {
		t.Errorf("custom SSL port redirect should include port:\n%s", contentCustom)
	}

	// 非 SSL 站点按端口监听
	plain := &model.Website{Domain: "plain.example.com", Port: 8080, Type: "static"}
	contentPlain, _, _ := svc.buildWebsiteNginxConfig(plain)
	if !strings.Contains(contentPlain, "listen 8080;") {
		t.Errorf("plain site should listen on 8080:\n%s", contentPlain)
	}
	if strings.Contains(contentPlain, "ssl http2") {
		t.Errorf("plain site must not contain ssl block:\n%s", contentPlain)
	}
}

package ssrf

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// isBlockedIP 判断 IP 是否属于内网/回环/链路本地/保留地址。
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// CGNAT 100.64.0.0/10
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

// ValidateHTTPURL 校验 URL 为 http/https 且目标解析后不是内网地址。
func ValidateHTTPURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("URL 非法: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https URL")
	}
	if u.Host == "" {
		return fmt.Errorf("URL 缺少主机")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("不允许访问内网地址")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("域名解析失败: %v", err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("域名解析到内网地址，已拒绝")
		}
	}
	return nil
}

// Fetch 带超时与大小上限地获取 URL 内容（SSRF 防护）。
func Fetch(raw string, maxBytes int64, timeout time.Duration) ([]byte, error) {
	if err := ValidateHTTPURL(raw); err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("响应体超过大小限制")
	}
	return data, nil
}

package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/repository"
)

func getSetting(key string) string {
	if global.DB == nil {
		return ""
	}
	repo := repository.NewSettingRepository(global.DB)
	item, err := repo.Get(key)
	if err != nil || item.Value == "" {
		return ""
	}
	return item.Value
}

func BindDomainMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		domain := getSetting("BindDomain")
		if domain == "" {
			c.Next()
			return
		}
		host := c.Request.Host
		if colonIdx := strings.Index(host, ":"); colonIdx != -1 {
			host = host[:colonIdx]
		}
		if host != domain {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func AllowIPsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ips := getSetting("AllowIPs")
		if ips == "" {
			c.Next()
			return
		}
		clientIP := c.ClientIP()

		if isPrivateIP(clientIP) {
			c.Next()
			return
		}

		for _, allowed := range strings.Split(ips, ",") {
			allowed = strings.TrimSpace(allowed)
			if allowed == "" {
				continue
			}
			if allowed == clientIP {
				c.Next()
				return
			}
			if strings.Contains(allowed, "/") {
				_, cidr, err := net.ParseCIDR(allowed)
				if err == nil && cidr.Contains(net.ParseIP(clientIP)) {
					c.Next()
					return
				}
			}
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "ip not allowed"})
		c.Abort()
	}
}

func isPrivateIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return true
	}
	return false
}
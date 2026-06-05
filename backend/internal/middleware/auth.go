package middleware

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/repository"
)

func SecurityEntranceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		entrance := getSecurityEntrance()
		if entrance == "" {
			c.Next()
			return
		}
		trimmed := strings.TrimSuffix(path, "/")
		if trimmed == "/"+entrance {
			c.SetCookie("SecurityEntrance", base64.StdEncoding.EncodeToString([]byte(entrance)), 0, "/", "", false, true)
			c.Redirect(http.StatusFound, "/dashboard")
			c.Abort()
			return
		}
		cookieVal, err := c.Cookie("SecurityEntrance")
		if err != nil {
			c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(notFoundPage))
			c.Abort()
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(cookieVal)
		if err != nil || string(decoded) != entrance {
			c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(notFoundPage))
			c.Abort()
			return
		}
		c.Next()
	}
}

func getSecurityEntrance() string {
	if global.DB == nil {
		return ""
	}
	repo := repository.NewSettingRepository(global.DB)
	item, err := repo.Get("SecurityEntrance")
	if err != nil || item.Value == "" {
		return ""
	}
	return item.Value
}

const notFoundPage = `<!DOCTYPE html><html><head><title>404</title></head><body><h1>404 - Not Found</h1></body></html>`

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			token := c.Query("token")
			if token != "" {
				auth = "Bearer " + token
			}
		}
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(global.CONF.JwtSecret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user", claims["user"])
			c.Set("role", claims["role"])
		}
		c.Next()
	}
}

func GenerateToken(username, role string) (string, error) {
	claims := jwt.MapClaims{
		"user": username,
		"role": role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(global.CONF.JwtSecret))
}

func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/permission"
	"github.com/minipanel/minipanel/internal/service"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // 非浏览器客户端
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	},
}

type TerminalAPI struct{}

func NewTerminalAPI() *TerminalAPI {
	return &TerminalAPI{}
}

func (a *TerminalAPI) HandleWS(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	if auth == "" {
		// 允许连接后通过首条 JSON 消息 {type:"auth", token:"..."} 鉴权
		auth = ""
	}

	id := c.Query("id")
	if id == "" {
		id = fmt.Sprintf("term_%d", c.Writer.Size())
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	tokenStr := ""
	if strings.HasPrefix(auth, "Bearer ") {
		tokenStr = strings.TrimPrefix(auth, "Bearer ")
	} else {
		// 浏览器 WebSocket 无法自定义 Header，改为连接后首条消息发送 token
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte("auth timeout"))
			return
		}
		var authMsg struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		if err := json.Unmarshal(data, &authMsg); err != nil || authMsg.Type != "auth" || authMsg.Token == "" {
			conn.WriteMessage(websocket.TextMessage, []byte("auth required"))
			return
		}
		tokenStr = authMsg.Token
		_ = conn.SetReadDeadline(time.Time{})
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		return []byte(global.CONF.JwtSecret), nil
	})
	if err != nil || !token.Valid {
		conn.WriteMessage(websocket.TextMessage, []byte("invalid token"))
		return
	}

	role := "user"
	var perms []string
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if r, ok := claims["role"].(string); ok {
			role = r
		}
		if raw, ok := claims["permissions"].([]interface{}); ok {
			for _, p := range raw {
				if s, ok := p.(string); ok {
					perms = append(perms, s)
				}
			}
		}
	}
	if !permission.HasFeature(role, perms, "/ssh") {
		conn.WriteMessage(websocket.TextMessage, []byte("permission denied"))
		return
	}

	if _, ok := service.GetSession(id); ok {
		service.RemoveSession(id)
	}

	shell := c.Query("shell")
	sess, err := service.NewTerminalSession(id, conn, shell, role)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v\n", err)))
		return
	}
	defer sess.Close()

	for !sess.IsClosed() {
		time.Sleep(100 * time.Millisecond)
	}
}

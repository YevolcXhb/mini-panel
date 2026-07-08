package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/service"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type TerminalAPI struct{}

func NewTerminalAPI() *TerminalAPI {
	return &TerminalAPI{}
}

func (a *TerminalAPI) HandleWS(c *gin.Context) {
	auth := c.Query("token")
	if auth == "" {
		auth = c.GetHeader("Authorization")
	}
	if auth == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
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

	id := c.Query("id")
	if id == "" {
		id = fmt.Sprintf("term_%d", c.Writer.Size())
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	role := "user"
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if r, ok := claims["role"].(string); ok {
			role = r
		}
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

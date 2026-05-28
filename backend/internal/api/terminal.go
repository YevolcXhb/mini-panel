package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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
	id := c.Query("id")
	if id == "" {
		id = fmt.Sprintf("term_%d", c.Writer.Size())
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	_, ok := service.GetSession(id)
	if ok {
		service.RemoveSession(id)
	}

	shell := c.Query("shell")
	sess, err := service.NewTerminalSession(id, conn, shell)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v\n", err)))
		return
	}
	defer sess.Close()

	// Block until session ends
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

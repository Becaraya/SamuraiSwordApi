package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WebSocketHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
}

var wsHub = WebSocketHub{
	clients: make(map[*websocket.Conn]bool),
}

func WebSocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	wsHub.mu.Lock()
	wsHub.clients[conn] = true
	wsHub.mu.Unlock()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			wsHub.mu.Lock()
			delete(wsHub.clients, conn)
			wsHub.mu.Unlock()
			break
		}
	}
}

func BroadcastMessage(message []byte) {
	wsHub.mu.Lock()
	defer wsHub.mu.Unlock()
	for client := range wsHub.clients {
		err := client.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			client.Close()
			delete(wsHub.clients, client)
		}
	}
}

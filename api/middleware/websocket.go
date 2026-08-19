package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Stockage des connexions WebSocket actives
	connections      = make(map[*websocket.Conn]string)
	connectionsMutex sync.RWMutex

	connectedUsers      = make(map[string]bool)
	connectedUsersMutex sync.RWMutex

	jwtSecret string
)

func SetJWTSecret(secret string) {
	jwtSecret = secret
}

// Message structure pour WebSocket
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
	From string      `json:"from,omitempty"`
}

// WebSocketHandler gère les connexions WebSocket
func WebSocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Erreur lors de l'upgrade WebSocket: %v", err)
		return
	}
	defer conn.Close()

	connectionsMutex.Lock()
	connections[conn] = ""
	connectionsMutex.Unlock()

	defer func() {
		connectionsMutex.Lock()
		username := connections[conn]
		delete(connections, conn)
		connectionsMutex.Unlock()

		if username != "" {
			connectedUsersMutex.Lock()
			delete(connectedUsers, username)
			connectedUsersMutex.Unlock()

			log.Printf("Utilisateur %s déconnecté", username)
		}
	}()

	for {
		var message WSMessage
		err := conn.ReadJSON(&message)
		if err != nil {
			log.Printf("Erreur lecture WebSocket: %v", err)
			break
		}

		handleWebSocketMessage(conn, message)
	}
}

func handleWebSocketMessage(conn *websocket.Conn, message WSMessage) {
	switch message.Type {
	case "auth":
		data, ok := message.Data.(map[string]interface{})
		if !ok {
			return
		}
		tokenString, ok := data["token"].(string)
		if !ok || tokenString == "" {
			log.Printf("Authentification WebSocket refusée: token manquant")
			return
		}
		claims, err := ValidateToken(tokenString, jwtSecret)
		if err != nil {
			log.Printf("Authentification WebSocket refusée: %v", err)
			return
		}

		username := claims.Username
		connectionsMutex.Lock()
		connections[conn] = username
		connectionsMutex.Unlock()

		log.Printf("Utilisateur %s connecté via WebSocket", username)

		connectedUsersMutex.Lock()
		connectedUsers[username] = true
		connectedUsersMutex.Unlock()

	case "join_game":
		if data, ok := message.Data.(map[string]interface{}); ok {
			if username, exists := data["username"]; exists {
				broadcastMessage := WSMessage{
					Type: "player_joined",
					Data: map[string]interface{}{
						"username": username,
						"message":  username.(string) + " a rejoint la partie!",
					},
					From: username.(string),
				}
				BroadcastToAll(broadcastMessage)
			}
		}

	case "leave_game":
		if data, ok := message.Data.(map[string]interface{}); ok {
			if username, exists := data["username"]; exists {
				broadcastMessage := WSMessage{
					Type: "player_left",
					Data: map[string]interface{}{
						"username": username,
						"message":  username.(string) + " a quitté la partie!",
					},
					From: username.(string),
				}
				BroadcastToAll(broadcastMessage)
			}
		}

	case "start_game":
		if data, ok := message.Data.(map[string]interface{}); ok {
			if username, exists := data["username"]; exists {
				broadcastMessage := WSMessage{
					Type: "game_starting",
					Data: map[string]interface{}{
						"username": username,
						"message":  "La partie va commencer...",
					},
					From: username.(string),
				}
				BroadcastToAll(broadcastMessage)
			}
		}
	}
}

func BroadcastToAll(message WSMessage) {
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()

	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Erreur lors de la sérialisation du message: %v", err)
		return
	}

	for conn := range connections {
		err := conn.WriteMessage(websocket.TextMessage, messageBytes)
		if err != nil {
			log.Printf("Erreur lors de l'envoi du message WebSocket: %v", err)
			delete(connections, conn)
		}
	}
}

func BroadcastMessage(messageBytes []byte) {
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()

	for conn := range connections {
		err := conn.WriteMessage(websocket.TextMessage, messageBytes)
		if err != nil {
			log.Printf("Erreur lors de l'envoi du message: %v", err)
			delete(connections, conn)
		}
	}
}

func GetActiveConnections() int {
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	return len(connections)
}

func GetConnectedUsernames() []string {
	connectedUsersMutex.RLock()
	defer connectedUsersMutex.RUnlock()

	usernames := make([]string, 0, len(connectedUsers))
	for username := range connectedUsers {
		usernames = append(usernames, username)
	}
	return usernames
}

package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	pingInterval = 20 * time.Second
	readTimeout  = 50 * time.Second // une connexion morte (réseau coupé) est détectée sans attendre le TCP
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

	onDisconnect func(username string)
	onConnect    func(username string)
)

func SetJWTSecret(secret string) {
	jwtSecret = secret
}

// SetDisconnectHandler définit le callback appelé quand un utilisateur perd sa dernière connexion.
func SetDisconnectHandler(handler func(username string)) {
	onDisconnect = handler
}

// SetConnectHandler définit le callback appelé quand un utilisateur s'authentifie sur le WebSocket.
func SetConnectHandler(handler func(username string)) {
	onConnect = handler
}

func IsUserConnected(username string) bool {
	connectedUsersMutex.RLock()
	defer connectedUsersMutex.RUnlock()
	return connectedUsers[username]
}

func broadcastPresence() {
	BroadcastToAll(WSMessage{
		Type: "presence",
		Data: map[string]interface{}{"connected_users": GetConnectedUsernames()},
	})
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

	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readTimeout))
	})
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			}
		}
	}()

	connectionsMutex.Lock()
	connections[conn] = ""
	connectionsMutex.Unlock()

	defer func() {
		connectionsMutex.Lock()
		username := connections[conn]
		delete(connections, conn)
		// Un rechargement de page peut ouvrir la nouvelle connexion avant la fermeture de l'ancienne.
		stillConnected := false
		for _, other := range connections {
			if other == username {
				stillConnected = true
				break
			}
		}
		connectionsMutex.Unlock()

		if username == "" || stillConnected {
			return
		}

		connectedUsersMutex.Lock()
		delete(connectedUsers, username)
		connectedUsersMutex.Unlock()

		log.Printf("Utilisateur %s déconnecté", username)
		broadcastPresence()
		if onDisconnect != nil {
			onDisconnect(username)
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
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
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

		broadcastPresence()
		if onConnect != nil {
			onConnect(username)
		}
	}
}

// BroadcastToAll envoie un message à toutes les connexions (Lock: les écritures WebSocket doivent être sérialisées).
func BroadcastToAll(message WSMessage) {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()

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
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()

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

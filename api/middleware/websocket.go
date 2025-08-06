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
            return true // Permettre toutes les origines pour le développement
        },
    }
    
    // Stockage des connexions WebSocket actives
    connections = make(map[*websocket.Conn]string)
    connectionsMutex sync.RWMutex
)

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

    // Ajouter la connexion à la liste
    connectionsMutex.Lock()
    connections[conn] = ""
    connectionsMutex.Unlock()

    // Nettoyer la connexion à la fermeture
    defer func() {
        connectionsMutex.Lock()
        delete(connections, conn)
        connectionsMutex.Unlock()
    }()

    // Écouter les messages entrants
    for {
        var message WSMessage
        err := conn.ReadJSON(&message)
        if err != nil {
            log.Printf("Erreur lecture WebSocket: %v", err)
            break
        }

        // Traiter le message selon son type
        handleWebSocketMessage(conn, message)
    }
}

// Traiter les messages WebSocket entrants
func handleWebSocketMessage(conn *websocket.Conn, message WSMessage) {
    switch message.Type {
    case "auth":
        // Authentifier l'utilisateur et associer le username à la connexion
        if data, ok := message.Data.(map[string]interface{}); ok {
            if username, exists := data["username"]; exists {
                connectionsMutex.Lock()
                connections[conn] = username.(string)
                connectionsMutex.Unlock()
                
                log.Printf("Utilisateur %s connecté via WebSocket", username)
            }
        }
    
    case "join_game":
        // Diffuser l'information que quelqu'un a rejoint
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
        // Diffuser l'information que quelqu'un a quitté
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
        // Diffuser le démarrage du jeu
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

// BroadcastToAll diffuse un message à toutes les connexions actives
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
            // Supprimer la connexion fermée
            delete(connections, conn)
        }
    }
}

// BroadcastMessage fonction utilitaire pour broadcaster des messages JSON
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

// GetActiveConnections retourne le nombre de connexions actives
func GetActiveConnections() int {
    connectionsMutex.RLock()
    defer connectionsMutex.RUnlock()
    return len(connections)
}

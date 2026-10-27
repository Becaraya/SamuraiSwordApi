package handler

import (
	"net/http"
	"time"

	"github.com/becaraya/katana-api/api/middleware"
	"github.com/becaraya/katana-api/internal/game"
	"github.com/gin-gonic/gin"
)

// disconnectGracePeriod laisse le temps à un joueur de recharger la page sans perdre sa place.
const disconnectGracePeriod = 15 * time.Second

// gameSnapshot construit l'état complet du lobby envoyé aux clients (HTTP et WebSocket).
func gameSnapshot() gin.H {
	currentGame := game.GetGameManager().GetCurrentGame()

	snapshot := gin.H{
		"game":            currentGame,
		"players":         make(map[string]*game.Player),
		"host":            "",
		"min_players":     game.MinPlayers,
		"max_players":     game.MaxPlayers,
		"connected_users": middleware.GetConnectedUsernames(),
	}
	if currentGame != nil {
		snapshot["players"] = currentGame.GetPlayers()
		snapshot["host"] = currentGame.Host()
	}
	return snapshot
}

func broadcastGameUpdate(event, username string) {
	data := gameSnapshot()
	data["event"] = event
	middleware.BroadcastToAll(middleware.WSMessage{Type: "game_update", Data: data, From: username})
}

// removePlayer retire un joueur de la partie courante et réinitialise le lobby s'il est vide.
func removePlayer(username string) bool {
	gameManager := game.GetGameManager()
	currentGame := gameManager.GetCurrentGame()
	if currentGame == nil || !currentGame.RemovePlayer(username) {
		return false
	}
	gameManager.ResetIfEmpty()
	return true
}

// HandleDisconnect retire un joueur du lobby s'il ne se reconnecte pas rapidement.
func HandleDisconnect(username string) {
	time.AfterFunc(disconnectGracePeriod, func() {
		if middleware.IsUserConnected(username) {
			return
		}
		currentGame := game.GetGameManager().GetCurrentGame()
		if currentGame == nil || currentGame.State != game.GameStateWaiting {
			return
		}
		if removePlayer(username) {
			broadcastGameUpdate("player_left", username)
		}
	})
}

func JoinGame(c *gin.Context) {
	username := c.GetString("username")
	gameManager := game.GetGameManager()

	if existingGame := gameManager.GetCurrentGame(); existingGame != nil {
		if player, alreadyIn := existingGame.GetPlayers()[username]; alreadyIn {
			c.JSON(http.StatusOK, gin.H{"message": "Already in game", "player": player, "game": existingGame})
			return
		}
		if existingGame.State != game.GameStateWaiting {
			c.JSON(http.StatusConflict, gin.H{"error": "game already in progress", "code": "GAME_IN_PROGRESS"})
			return
		}
	}

	player, success := gameManager.JoinCurrentGame(username)
	if !success {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not join game (full or already started)", "code": "JOIN_FAILED"})
		return
	}

	broadcastGameUpdate("player_joined", username)

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully joined game",
		"player":  player,
		"game":    gameManager.GetCurrentGame(),
	})
}

func LeaveGame(c *gin.Context) {
	username := c.GetString("username")

	if !removePlayer(username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player not in game"})
		return
	}

	broadcastGameUpdate("player_left", username)

	c.JSON(http.StatusOK, gin.H{"message": "Successfully left game"})
}

// GetGameState retourne l'état actuel du jeu
func GetGameState(c *gin.Context) {
	c.JSON(http.StatusOK, gameSnapshot())
}

// EndGame termine la partie lancée (réservé à l'hôte) et ramène les joueurs au lobby
func EndGame(c *gin.Context) {
	username := c.GetString("username")
	currentGame := game.GetGameManager().GetCurrentGame()

	if currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active game"})
		return
	}
	if _, inGame := currentGame.GetPlayers()[username]; !inGame {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not in this game"})
		return
	}
	if currentGame.Host() != username {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the host can end the game", "code": "NOT_HOST"})
		return
	}
	if !currentGame.EndGame() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Game is not started"})
		return
	}

	broadcastGameUpdate("game_ended", username)

	c.JSON(http.StatusOK, gin.H{"message": "Game ended", "game": currentGame})
}

// GetMyPrivateState retourne la main et le rôle secret du joueur courant
func GetMyPrivateState(c *gin.Context) {
	currentGame := game.GetGameManager().GetCurrentGame()
	if currentGame == nil {
		c.JSON(http.StatusOK, gin.H{"hand": []game.Card{}, "role": nil})
		return
	}
	username := c.GetString("username")
	c.JSON(http.StatusOK, gin.H{"hand": currentGame.HandOf(username), "role": currentGame.RoleOf(username)})
}

// StartGame démarre la partie (réservé à l'hôte)
func StartGame(c *gin.Context) {
	username := c.GetString("username")
	currentGame := game.GetGameManager().GetCurrentGame()

	if currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active game"})
		return
	}

	players := currentGame.GetPlayers()
	if _, inGame := players[username]; !inGame {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not in this game"})
		return
	}

	if currentGame.Host() != username {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the host can start the game", "code": "NOT_HOST"})
		return
	}

	if len(players) < game.MinPlayers {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":            "Minimum players required to start the game",
			"current_players":  len(players),
			"minimum_required": game.MinPlayers,
		})
		return
	}

	if !currentGame.StartGame() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot start game"})
		return
	}

	broadcastGameUpdate("game_started", username)

	c.JSON(http.StatusOK, gin.H{
		"message": "Game started successfully",
		"game":    currentGame,
	})
}

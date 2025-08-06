package handler

import (
    "encoding/json"
    "net/http"

    "github.com/becaraya/katana-api/api/middleware"
    "github.com/becaraya/katana-api/internal/game"
    "github.com/gin-gonic/gin"
)

type JoinGameRequest struct {
    Username string `json:"username" binding:"required"`
}

type LeaveGameRequest struct {
    Username string `json:"username" binding:"required"`
}

// JoinGame permet à un joueur de rejoindre la partie
func JoinGame(c *gin.Context) {
    var req JoinGameRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    gameManager := game.GetGameManager()
    player, success := gameManager.JoinCurrentGame(req.Username)

    if !success {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Could not join game"})
        return
    }

    // Broadcaster la mise à jour du jeu
    currentGame := gameManager.GetCurrentGame()
    message, _ := json.Marshal(map[string]interface{}{
        "type":    "game_update",
        "game":    currentGame,
        "players": currentGame.GetPlayers(),
    })
    middleware.BroadcastMessage(message)

    c.JSON(http.StatusOK, gin.H{
        "message": "Successfully joined game",
        "player":  player,
        "game":    currentGame,
    })
}

func LeaveGame(c *gin.Context) {
    var req LeaveGameRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    gameManager := game.GetGameManager()
    currentGame := gameManager.GetCurrentGame()

    if currentGame == nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No active game"})
        return
    }

    success := currentGame.RemovePlayer(req.Username)
    if !success {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Player not in game"})
        return
    }

    // Broadcaster la mise à jour du jeu
    broadcastMessage := middleware.WSMessage{
        Type: "game_update",
        Data: map[string]interface{}{
            "game":    currentGame,
            "players": currentGame.GetPlayers(),
            "event":   "player_left",
            "message": req.Username + " a quitté la partie!",
        },
        From: req.Username,
    }
    middleware.BroadcastToAll(broadcastMessage)

    c.JSON(http.StatusOK, gin.H{
        "message": "Successfully left game",
        "game":    currentGame,
        "players": currentGame.GetPlayers(),
    })
}

// GetGameState retourne l'état actuel du jeu
func GetGameState(c *gin.Context) {
    gameManager := game.GetGameManager()
    currentGame := gameManager.GetCurrentGame()

    if currentGame == nil {
        c.JSON(http.StatusOK, gin.H{
            "game":    nil,
            "players": make(map[string]*game.Player),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "game":    currentGame,
        "players": currentGame.GetPlayers(),
    })
}

// StartGame démarre la partie
func StartGame(c *gin.Context) {
    gameManager := game.GetGameManager()
    currentGame := gameManager.GetCurrentGame()

    if currentGame == nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No active game"})
        return
    }

    if !currentGame.StartGame() {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot start game"})
        return
    }

    // Broadcaster le démarrage du jeu
    message, _ := json.Marshal(map[string]interface{}{
        "type": "game_started",
        "game": currentGame,
    })
    middleware.BroadcastMessage(message)

    c.JSON(http.StatusOK, gin.H{
        "message": "Game started successfully",
        "game":    currentGame,
    })
}
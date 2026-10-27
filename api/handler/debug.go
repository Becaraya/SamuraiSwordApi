package handler

import (
	"net/http"
	"strconv"

	"github.com/becaraya/katana-api/internal/game"
	"github.com/gin-gonic/gin"
)

// DebugCreateGame remplace la partie courante par une partie de test (joueur courant + bots).
// Query : players (3-7, défaut 3), start (true pour lancer directement la partie).
func DebugCreateGame(c *gin.Context) {
	username := c.GetString("username")

	players := game.MinPlayers
	if raw := c.Query("players"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < game.MinPlayers || n > game.MaxPlayers {
			c.JSON(http.StatusBadRequest, gin.H{"error": "players must be between " + strconv.Itoa(game.MinPlayers) + " and " + strconv.Itoa(game.MaxPlayers)})
			return
		}
		players = n
	}
	start, _ := strconv.ParseBool(c.Query("start"))

	g := game.GetGameManager().CreateDebugGame(username, players, start)

	event := "player_joined"
	if start {
		event = "game_started"
	}
	broadcastGameUpdate(event, username)
	scheduleBotTurns()

	c.JSON(http.StatusOK, gin.H{"message": "Debug game created", "game": g, "players": g.GetPlayers()})
}

// DebugAddBot ajoute un bot à la partie courante tant qu'elle n'est pas lancée ni pleine.
func DebugAddBot(c *gin.Context) {
	bot, ok := game.GetGameManager().AddDebugBot()
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot add bot (no lobby, full or already started)"})
		return
	}

	broadcastGameUpdate("player_joined", bot.Name)

	c.JSON(http.StatusOK, gin.H{"message": "Bot added", "player": bot})
}

// DebugDeleteGame supprime la partie courante (bots inclus).
func DebugDeleteGame(c *gin.Context) {
	game.GetGameManager().DeleteCurrentGame()
	broadcastGameUpdate("game_deleted", c.GetString("username"))
	c.JSON(http.StatusOK, gin.H{"message": "Game deleted"})
}

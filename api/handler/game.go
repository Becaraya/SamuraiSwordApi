package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/becaraya/katana-api/api/middleware"
	"github.com/becaraya/katana-api/internal/game"
	"github.com/gin-gonic/gin"
)

// disconnectGracePeriod laisse le temps à un joueur de recharger la page sans perdre sa place au lobby.
const disconnectGracePeriod = 15 * time.Second

// awayDelay et abandonDelay : en partie, délai avant la prise en main par le serveur, puis avant l'abandon si plus aucun humain.
const (
	awayDelay    = 30 * time.Second
	abandonDelay = 2 * time.Minute
)

// botTurnDelay laisse voir le tour d'un bot (mode debug) avant qu'il ne passe la main.
const botTurnDelay = 1500 * time.Millisecond

// scheduleBotTurns enchaîne les actions des bots jusqu'à ce qu'un joueur humain doive agir.
func scheduleBotTurns() {
	currentGame := game.GetGameManager().GetCurrentGame()
	if currentGame == nil || !currentGame.NeedsBotStep() {
		return
	}
	time.AfterFunc(botTurnDelay, func() {
		if name, ok := currentGame.PlayBotStep(); ok {
			broadcastGameUpdate("bot_action", name)
			scheduleBotTurns()
		}
	})
}

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

// removePlayer retire un joueur du lobby (réinitialisé s'il est vide) ou, en pleine partie, le passe en mode automatique.
// Retourne l'événement à diffuser, ou "" s'il n'y a rien à signaler.
func removePlayer(username string) string {
	gameManager := game.GetGameManager()
	currentGame := gameManager.GetCurrentGame()
	if currentGame == nil {
		return ""
	}
	if currentGame.Status() != game.GameStateWaiting {
		if !currentGame.SetAway(username, true) {
			return ""
		}
		scheduleBotTurns()
		scheduleAbandon(currentGame)
		return "player_away"
	}
	if !currentGame.RemovePlayer(username) {
		return ""
	}
	gameManager.ResetIfEmpty()
	return "player_left"
}

// scheduleAbandon supprime une partie commencée dont tous les humains sont partis.
func scheduleAbandon(g *game.Game) {
	if !g.AllHumansAway() {
		return
	}
	time.AfterFunc(abandonDelay, func() {
		gm := game.GetGameManager()
		if gm.GetCurrentGame() != g || !g.AllHumansAway() {
			return
		}
		gm.DeleteCurrentGame()
		broadcastGameUpdate("game_abandoned", "")
	})
}

// HandleDisconnect traite une déconnexion : au lobby le joueur perd sa place après un délai de grâce,
// en partie il est joué automatiquement après awayDelay (il garde sa place).
func HandleDisconnect(username string) {
	delay := disconnectGracePeriod
	if cg := game.GetGameManager().GetCurrentGame(); cg != nil && cg.Status() != game.GameStateWaiting {
		delay = awayDelay
	}
	time.AfterFunc(delay, func() {
		if middleware.IsUserConnected(username) {
			return
		}
		if event := removePlayer(username); event != "" {
			broadcastGameUpdate(event, username)
		}
	})
}

// HandleConnect rend la main à un joueur qui se reconnecte en pleine partie.
func HandleConnect(username string) {
	if cg := game.GetGameManager().GetCurrentGame(); cg != nil && cg.SetAway(username, false) {
		broadcastGameUpdate("player_back", username)
	}
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

	event := removePlayer(username)
	if event == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player not in game"})
		return
	}
	broadcastGameUpdate(event, username)

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

// SetReady marque le joueur courant comme prêt (ou non) dans le lobby
func SetReady(c *gin.Context) {
	username := c.GetString("username")
	var body struct {
		Ready bool `json:"ready"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}

	currentGame := game.GetGameManager().GetCurrentGame()
	if currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active game"})
		return
	}
	if _, inGame := currentGame.GetPlayers()[username]; !inGame {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not in this game"})
		return
	}
	if err := currentGame.SetReady(username, body.Ready); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "GAME_IN_PROGRESS"})
		return
	}

	broadcastGameUpdate("player_ready", username)
	c.JSON(http.StatusOK, gin.H{"message": "Ready state updated", "ready": body.Ready})
}

// turnError traduit une erreur de tour en réponse HTTP
func turnError(c *gin.Context, err error) {
	status, code := http.StatusBadRequest, "INVALID_ACTION"
	switch err {
	case game.ErrNotYourTurn:
		status, code = http.StatusForbidden, "NOT_YOUR_TURN"
	case game.ErrWrongPhase:
		status, code = http.StatusConflict, "WRONG_PHASE"
	case game.ErrNotStarted:
		status, code = http.StatusConflict, "NOT_STARTED"
	case game.ErrInvalidDiscard:
		code = "INVALID_DISCARD"
	case game.ErrAttackPending:
		status, code = http.StatusConflict, "ATTACK_PENDING"
	case game.ErrNoAttack:
		status, code = http.StatusConflict, "NO_ATTACK"
	case game.ErrNotTarget:
		status, code = http.StatusForbidden, "NOT_TARGET"
	case game.ErrNotPlayable:
		status, code = http.StatusConflict, "NOT_PLAYABLE"
	case game.ErrInvalidTarget:
		code = "INVALID_TARGET"
	case game.ErrInvalidParry:
		code = "INVALID_PARRY"
	case game.ErrCardNotFound:
		status, code = http.StatusNotFound, "CARD_NOT_FOUND"
	}
	c.JSON(status, gin.H{"error": err.Error(), "code": code})
}

// EndTurn termine la phase de jeu du joueur actif
func EndTurn(c *gin.Context) {
	username := c.GetString("username")
	currentGame := game.GetGameManager().GetCurrentGame()
	if currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active game"})
		return
	}
	if err := currentGame.EndTurn(username); err != nil {
		turnError(c, err)
		return
	}

	broadcastGameUpdate("turn_changed", username)
	scheduleBotTurns()
	c.JSON(http.StatusOK, gin.H{"message": "Turn ended", "game": currentGame})
}

// DiscardCards défausse les cartes exigées en fin de tour (indices dans la main)
func DiscardCards(c *gin.Context) {
	username := c.GetString("username")
	var body struct {
		Cards []int `json:"cards"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}

	currentGame := game.GetGameManager().GetCurrentGame()
	if currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active game"})
		return
	}
	if err := currentGame.DiscardCards(username, body.Cards); err != nil {
		turnError(c, err)
		return
	}

	broadcastGameUpdate("turn_changed", username)
	scheduleBotTurns()
	c.JSON(http.StatusOK, gin.H{"message": "Cards discarded", "game": currentGame})
}

// GetMyPrivateState retourne la main, le rôle secret et les actions possibles du joueur courant
func GetMyPrivateState(c *gin.Context) {
	currentGame := game.GetGameManager().GetCurrentGame()
	if currentGame == nil {
		c.JSON(http.StatusOK, game.PrivateState{Hand: []game.Card{}, Plays: []game.Play{}, ResponseCards: []int{}})
		return
	}
	c.JSON(http.StatusOK, currentGame.PrivateState(c.GetString("username")))
}

// GetTargets liste les cibles possibles de l'arme à l'indice `card` de la main
func GetTargets(c *gin.Context) {
	index, err := strconv.Atoi(c.Query("card"))
	currentGame := game.GetGameManager().GetCurrentGame()
	if err != nil || currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	targets, err := currentGame.TargetsFor(c.GetString("username"), index)
	if err != nil {
		turnError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"targets": targets})
}

// PlayWeapon joue la carte à l'indice `card` (cible et option facultatives selon la carte)
func PlayWeapon(c *gin.Context) {
	username := c.GetString("username")
	var body struct {
		Card   int    `json:"card"`
		CardID string `json:"card_id"`
		Target string `json:"target"`
		Option string `json:"option"`
	}
	currentGame := game.GetGameManager().GetCurrentGame()
	if err := c.ShouldBindJSON(&body); err != nil || currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if err := currentGame.PlayCardExpect(username, body.Card, body.CardID, body.Target, body.Option); err != nil {
		turnError(c, err)
		return
	}

	broadcastGameUpdate("attack", username)
	scheduleBotTurns()
	c.JSON(http.StatusOK, gin.H{"message": "Card played", "game": currentGame})
}

// GetCatalog retourne toutes les cartes et tous les personnages (données publiques).
func GetCatalog(c *gin.Context) {
	cards, characters := game.Catalog()
	c.JSON(http.StatusOK, gin.H{"cards": cards, "characters": characters})
}

// PostChat publie un message dans le chat de la table.
func PostChat(c *gin.Context) {
	var body struct {
		Text string `json:"text"`
	}
	currentGame := game.GetGameManager().GetCurrentGame()
	if err := c.ShouldBindJSON(&body); err != nil || currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	username := c.GetString("username")
	switch err := currentGame.PostChat(username, body.Text); err {
	case nil:
		broadcastGameUpdate("chat", username)
		c.JSON(http.StatusOK, gin.H{"message": "Sent"})
	case game.ErrChatRate:
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error(), "code": "CHAT_RATE"})
	case game.ErrNotInGame:
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error(), "code": "NOT_IN_GAME"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "CHAT_INVALID"})
	}
}

// UsePower active le pouvoir volontaire du personnage du joueur courant
func UsePower(c *gin.Context) {
	username := c.GetString("username")
	var body struct {
		Power string `json:"power"`
	}
	currentGame := game.GetGameManager().GetCurrentGame()
	if err := c.ShouldBindJSON(&body); err != nil || currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if err := currentGame.UsePower(username, body.Power); err != nil {
		turnError(c, err)
		return
	}

	broadcastGameUpdate("power", username)
	c.JSON(http.StatusOK, gin.H{"message": "Power used", "game": currentGame})
}

// RespondToAttack permet à la cible de parer (carte `card`) ou de subir l'attaque
func RespondToAttack(c *gin.Context) {
	username := c.GetString("username")
	var body struct {
		Parry  bool   `json:"parry"`
		Card   int    `json:"card"`
		CardID string `json:"card_id"`
	}
	currentGame := game.GetGameManager().GetCurrentGame()
	if err := c.ShouldBindJSON(&body); err != nil || currentGame == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if err := currentGame.RespondExpect(username, body.Parry, body.Card, body.CardID); err != nil {
		turnError(c, err)
		return
	}

	broadcastGameUpdate("respond", username)
	scheduleBotTurns()
	c.JSON(http.StatusOK, gin.H{"message": "Response recorded", "game": currentGame})
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

	if !currentGame.AllReady(username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "All players must be ready", "code": "NOT_READY"})
		return
	}

	if !currentGame.StartGame() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot start game"})
		return
	}

	broadcastGameUpdate("game_started", username)
	scheduleBotTurns()

	c.JSON(http.StatusOK, gin.H{
		"message": "Game started successfully",
		"game":    currentGame,
	})
}

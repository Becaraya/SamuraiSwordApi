package game

import (
	"fmt"
	"sync"
	"time"
)

// GameManager gère toutes les parties en cours
type GameManager struct {
	games   map[string]*Game
	mu      sync.RWMutex
	current *Game
}

var (
	instance *GameManager
	once     sync.Once
)

// GetGameManager retourne l'instance singleton du gestionnaire
func GetGameManager() *GameManager {
	once.Do(func() {
		instance = &GameManager{
			games: make(map[string]*Game),
		}
	})
	return instance
}

// CreateGame crée une nouvelle partie
func (gm *GameManager) CreateGame(createdBy string) *Game {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	game := NewGame(createdBy)
	gm.games[game.ID] = game
	gm.current = game
	return game
}

// GetCurrentGame retourne la partie actuelle
func (gm *GameManager) GetCurrentGame() *Game {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.current
}

// GetOrCreateCurrentGame retourne la partie actuelle ou en crée une
func (gm *GameManager) GetOrCreateCurrentGame(createdBy string) *Game {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.current == nil {
		gm.current = NewGame(createdBy)
		gm.games[gm.current.ID] = gm.current
	}
	return gm.current
}

// ResetIfEmpty supprime la partie courante si plus aucun joueur n'y est présent
func (gm *GameManager) ResetIfEmpty() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.current != nil && len(gm.current.GetPlayers()) == 0 {
		delete(gm.games, gm.current.ID)
		gm.current = nil
	}
}

// CreateDebugGame remplace la partie courante par une partie de test : l'hôte réel + des bots jusqu'à `total` joueurs.
func (gm *GameManager) CreateDebugGame(host string, total int, start bool) *Game {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if total < MinPlayers {
		total = MinPlayers
	}
	if total > MaxPlayers {
		total = MaxPlayers
	}

	if gm.current != nil {
		delete(gm.games, gm.current.ID)
	}

	g := NewGame(host)
	now := time.Now()

	// JoinedAt croissant pour que l'hôte réel reste le premier arrivé.
	hostPlayer := NewPlayer(host, 0)
	hostPlayer.JoinedAt = now
	g.AddPlayer(hostPlayer)

	for i := 1; i < total; i++ {
		bot := NewPlayer(fmt.Sprintf("%s%d", BotPrefix, i), 0)
		bot.JoinedAt = now.Add(time.Duration(i) * time.Millisecond)
		g.AddPlayer(bot)
	}

	if start {
		g.StartGame()
	}

	gm.games[g.ID] = g
	gm.current = g
	return g
}

// DeleteCurrentGame supprime la partie courante
func (gm *GameManager) DeleteCurrentGame() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.current != nil {
		delete(gm.games, gm.current.ID)
		gm.current = nil
	}
}

// JoinCurrentGame fait rejoindre un joueur à la partie actuelle
func (gm *GameManager) JoinCurrentGame(playerName string) (*Player, bool) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if gm.current == nil {
		gm.current = NewGame(playerName)
		gm.games[gm.current.ID] = gm.current
	}

	player := NewPlayer(playerName, 0)
	success := gm.current.AddPlayer(player)
	if success {
		return player, true
	}
	return nil, false
}

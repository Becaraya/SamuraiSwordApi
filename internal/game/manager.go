package game

import (
    "sync"
)

// GameManager gère toutes les parties en cours
type GameManager struct {
    games   map[string]*Game
    mu      sync.RWMutex
    current *Game // Partie principale (pour simplifier, une seule partie active)
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
    gm.current = game // Définir comme partie actuelle
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

// JoinCurrentGame fait rejoindre un joueur à la partie actuelle
func (gm *GameManager) JoinCurrentGame(playerName string) (*Player, bool) {
    gm.mu.Lock()
    defer gm.mu.Unlock()

    if gm.current == nil {
        gm.current = NewGame(playerName)
        gm.games[gm.current.ID] = gm.current
    }

    player := NewPlayer(playerName, 0) // Position sera assignée automatiquement
    success := gm.current.AddPlayer(player)
    if success {
        return player, true
    }
    return nil, false
}
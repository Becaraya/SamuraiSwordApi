package game

import (
    "sync"
    "time"
)

// GameState représente l'état actuel du jeu
type GameState string

const (
    GameStateWaiting GameState = "WAITING"
    GameStateStarted GameState = "STARTED"
    GameStatePaused  GameState = "PAUSED"
    GameStateEnded   GameState = "ENDED"
)

// Player représente un joueur dans le jeu
type Player struct {
    Name     string `json:"name"`
    Position int    `json:"position"`
    Life     int    `json:"life"`
    Honor    int    `json:"honor"`
    JoinedAt time.Time `json:"joined_at"`
}

// Game représente l'état complet d'une partie
type Game struct {
    ID          string            `json:"id"`
    State       GameState         `json:"state"`
    Players     map[string]*Player `json:"players"`
    CreatedBy   string            `json:"created_by"`
    CreatedAt   time.Time         `json:"created_at"`
    MaxPlayers  int               `json:"max_players"`
    mu          sync.RWMutex
}

// NewGame crée une nouvelle partie
func NewGame(createdBy string) *Game {
    return &Game{
        ID:         generateGameID(),
        State:      GameStateWaiting,
        Players:    make(map[string]*Player),
        CreatedBy:  createdBy,
        CreatedAt:  time.Now(),
        MaxPlayers: 7, // Maximum 7 joueurs selon votre interface
    }
}

// NewPlayer crée un nouveau joueur avec les valeurs par défaut
func NewPlayer(name string, position int) *Player {
    return &Player{
        Name:     name,
        Position: position,
        Life:     4, // Valeur par défaut
        Honor:    4, // Valeur par défaut
        JoinedAt: time.Now(),
    }
}

// AddPlayer ajoute un joueur à la partie
func (g *Game) AddPlayer(player *Player) bool {
    g.mu.Lock()
    defer g.mu.Unlock()

    // Vérifier si la partie n'est pas pleine
    if len(g.Players) >= g.MaxPlayers {
        return false
    }

    // Vérifier si le joueur n'est pas déjà dans la partie
    if _, exists := g.Players[player.Name]; exists {
        return false
    }

    // Attribuer la prochaine position disponible
    player.Position = g.getNextPosition()
    g.Players[player.Name] = player
    return true
}

// RemovePlayer retire un joueur de la partie
func (g *Game) RemovePlayer(playerName string) bool {
    g.mu.Lock()
    defer g.mu.Unlock()

    if _, exists := g.Players[playerName]; exists {
        delete(g.Players, playerName)
        return true
    }
    return false
}

// GetPlayers retourne la liste des joueurs
func (g *Game) GetPlayers() map[string]*Player {
    g.mu.RLock()
    defer g.mu.RUnlock()

    // Créer une copie pour éviter les race conditions
    players := make(map[string]*Player)
    for name, player := range g.Players {
        players[name] = player
    }
    return players
}

// StartGame démarre la partie
func (g *Game) StartGame() bool {
    g.mu.Lock()
    defer g.mu.Unlock()

    if g.State == GameStateWaiting && len(g.Players) >= 2 {
        g.State = GameStateStarted
        return true
    }
    return false
}

// getNextPosition trouve la prochaine position disponible
func (g *Game) getNextPosition() int {
    usedPositions := make(map[int]bool)
    for _, player := range g.Players {
        usedPositions[player.Position] = true
    }

    for i := 1; i <= g.MaxPlayers; i++ {
        if !usedPositions[i] {
            return i
        }
    }
    return 1 // Par défaut
}

// generateGameID génère un ID unique pour la partie
func generateGameID() string {
    return time.Now().Format("20060102150405")
}
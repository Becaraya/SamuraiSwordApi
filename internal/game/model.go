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

const MinPlayers = 3
const MaxPlayers = 7

// BotPrefix préfixe les joueurs fictifs créés par le mode debug
const BotPrefix = "Bot-"

// Player représente un joueur dans le jeu
type Player struct {
	Name       string     `json:"name"`
	Position   int        `json:"position"`
	Life       int        `json:"life"`
	MaxLife    int        `json:"max_life"`
	Honor      int        `json:"honor"`
	JoinedAt   time.Time  `json:"joined_at"`
	Character  *Character `json:"character"`
	Permanents []Card     `json:"permanents"`
	HandCount  int        `json:"hand_count"`
	Hand       []Card     `json:"-"` // privée : exposée uniquement à son propriétaire
}

// Game représente l'état complet d'une partie
type Game struct {
	ID         string             `json:"id"`
	State      GameState          `json:"state"`
	Players    map[string]*Player `json:"players"`
	CreatedBy  string             `json:"created_by"`
	CreatedAt  time.Time          `json:"created_at"`
	MaxPlayers int                `json:"max_players"`

	Deck         []Card `json:"-"`
	Discard      []Card `json:"-"`
	DeckCount    int    `json:"deck_count"`
	DiscardCount int    `json:"discard_count"`
	DiscardTop   *Card  `json:"discard_top"`

	mu sync.RWMutex
}

// NewGame crée une nouvelle partie
func NewGame(createdBy string) *Game {
	return &Game{
		ID:         generateGameID(),
		State:      GameStateWaiting,
		Players:    make(map[string]*Player),
		CreatedBy:  createdBy,
		CreatedAt:  time.Now(),
		MaxPlayers: MaxPlayers,
	}
}

// NewPlayer crée un nouveau joueur avec les valeurs par défaut
func NewPlayer(name string, position int) *Player {
	return &Player{
		Name:     name,
		Position: position,
		Life:       4, // Valeur par défaut
		MaxLife:    4,
		Honor:      4, // Valeur par défaut
		JoinedAt:   time.Now(),
		Permanents: []Card{},
	}
}

// AddPlayer ajoute un joueur à la partie
func (g *Game) AddPlayer(player *Player) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	// On ne peut rejoindre que si la partie n'est pas déjà lancée
	if g.State != GameStateWaiting {
		return false
	}

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

// Host retourne le joueur arrivé en premier (l'hôte du lobby), ou "" si la partie est vide
func (g *Game) Host() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	host := ""
	var earliest time.Time
	for name, player := range g.Players {
		if host == "" || player.JoinedAt.Before(earliest) {
			host = name
			earliest = player.JoinedAt
		}
	}
	return host
}

// StartGame démarre la partie
func (g *Game) StartGame() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State == GameStateWaiting && len(g.Players) >= MinPlayers {
		g.State = GameStateStarted
		g.deal()
		return true
	}
	return false
}

// EndGame termine la partie lancée et ramène les joueurs au lobby
func (g *Game) EndGame() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State != GameStateStarted {
		return false
	}

	g.State = GameStateWaiting
	g.Deck = nil
	g.Discard = nil
	g.syncPiles()
	for _, player := range g.Players {
		defaults := NewPlayer(player.Name, player.Position)
		player.Character = nil
		player.Hand = nil
		player.HandCount = 0
		player.Permanents = defaults.Permanents
		player.Life = defaults.Life
		player.MaxLife = defaults.MaxLife
	}
	return true
}

// deal attribue un personnage et une main de départ à chaque joueur (verrou déjà pris).
func (g *Game) deal() {
	g.Deck = buildDeck()
	g.Discard = nil

	chars := shuffledCharacters()
	i := 0
	for _, player := range g.Players {
		character := chars[i%len(chars)]
		i++
		player.Character = &character
		player.Life = character.Life
		player.MaxLife = character.Life
		player.Permanents = []Card{}
		player.Hand = append([]Card(nil), g.Deck[:StartingHandSize]...)
		player.HandCount = len(player.Hand)
		g.Deck = g.Deck[StartingHandSize:]
	}

	g.syncPiles()
}

// syncPiles met à jour les compteurs publics de la pioche et de la défausse.
func (g *Game) syncPiles() {
	g.DeckCount = len(g.Deck)
	g.DiscardCount = len(g.Discard)
	g.DiscardTop = nil
	if n := len(g.Discard); n > 0 {
		top := g.Discard[n-1]
		g.DiscardTop = &top
	}
}

// HandOf retourne une copie de la main d'un joueur.
func (g *Game) HandOf(playerName string) []Card {
	g.mu.RLock()
	defer g.mu.RUnlock()

	hand := []Card{}
	if player, ok := g.Players[playerName]; ok {
		hand = append(hand, player.Hand...)
	}
	return hand
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

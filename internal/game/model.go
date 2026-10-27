package game

import (
	"strings"
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
	Role       *Role      `json:"-"`    // privé : exposé uniquement à son propriétaire
	PublicRole *Role      `json:"role"` // renseigné seulement pour le Shogun
	Permanents []Card     `json:"permanents"`
	HandCount  int        `json:"hand_count"`
	Down       string     `json:"down"` // "", "hp" ou "hand" : joueur à terre (pas éliminé)
	Ready      bool       `json:"ready"`
	Away       bool       `json:"away"` // déconnecté en pleine partie : le serveur joue à sa place
	Hand       []Card     `json:"-"`    // privée : exposée uniquement à son propriétaire
}

// Game représente l'état complet d'une partie
type Game struct {
	ID         string             `json:"id"`
	State      GameState          `json:"state"`
	Players    map[string]*Player `json:"players"`
	CreatedBy  string             `json:"created_by"`
	CreatedAt  time.Time          `json:"created_at"`
	MaxPlayers int                `json:"max_players"`

	Active      string         `json:"active"` // joueur dont c'est le tour
	Phase       Phase          `json:"phase"`
	Turn        int            `json:"turn"`
	Round       int            `json:"round"`  // numéro de la partie dans ce lobby
	Played      int            `json:"played"` // armes jouées ce tour
	Pending     *Attack        `json:"pending"`
	Collective  *Collective    `json:"collective"`
	Bushido     *BushidoChoice `json:"bushido"`
	DrawChoice  *DrawChoice    `json:"draw_choice"`
	Results     *Results       `json:"results,omitempty"`
	Log         []LogEntry     `json:"log"`
	EndedReason string         `json:"ended_reason,omitempty"`
	Chat        []ChatMessage  `json:"chat"`

	Deck         []Card `json:"-"`
	Discard      []Card `json:"-"`
	DeckCount    int    `json:"deck_count"`
	DiscardCount int    `json:"discard_count"`
	DiscardTop   *Card  `json:"discard_top"`

	lastDefeat *defeat // dernière défaite en cours de traitement (pour Lame perdue et victoire au fil de l'épée)
	sword      string  // joueur vainqueur au fil de l'épée

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
		Name:       name,
		Position:   position,
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

// Status retourne l'état de la partie (lecture protégée par le verrou).
func (g *Game) Status() GameState {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.State
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

// StartGame démarre la partie (la vérification des joueurs prêts est faite par le handler)
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

// EndGame termine la partie (lancée ou terminée) et ramène les joueurs au lobby
func (g *Game) EndGame() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State != GameStateStarted && g.State != GameStateEnded {
		return false
	}

	g.State = GameStateWaiting
	g.Deck = nil
	g.Discard = nil
	g.Active = ""
	g.Phase = ""
	g.Turn = 0
	g.DrawChoice = nil
	g.Played = 0
	g.Pending = nil
	g.Collective = nil
	g.Bushido = nil
	g.Log = nil
	g.EndedReason = ""
	g.Results = nil
	g.Chat = nil
	g.lastDefeat, g.sword = nil, ""
	g.syncPiles()
	for _, player := range g.Players {
		defaults := NewPlayer(player.Name, player.Position)
		player.Character = nil
		player.Role = nil
		player.PublicRole = nil
		player.Hand = nil
		player.HandCount = 0
		player.Down = ""
		player.Away = false
		player.Ready = strings.HasPrefix(player.Name, BotPrefix)
		player.Permanents = defaults.Permanents
		player.Life = defaults.Life
		player.MaxLife = defaults.MaxLife
		player.Honor = defaults.Honor
	}
	return true
}

// deal attribue personnage, rôle, honneur et main de départ, puis lance le tour du Shogun (verrou déjà pris).
func (g *Game) deal() {
	g.Deck = buildDeck()
	g.Discard = nil

	players := g.sortedPlayers()
	chars := shuffledCharacters()
	dealtRoles := shuffledRoles(len(players))
	shogunHonor, othersHonor := startingHonor(len(players))

	shogunIdx := 0
	for i, player := range players {
		character := chars[i%len(chars)]
		role := dealtRoles[i%len(dealtRoles)]
		player.Character = &character
		player.Role = &role
		player.PublicRole = nil
		player.Honor = othersHonor
		if role.ID == RoleShogun {
			player.PublicRole = &role
			player.Honor = shogunHonor
			shogunIdx = i
		}
		player.Life = character.Life
		player.MaxLife = character.Life
		player.Permanents = []Card{}
		player.Hand = nil
	}

	// La main de départ dépend de l'ordre de jeu, à partir du Shogun.
	for k := range players {
		player := players[(shogunIdx+k)%len(players)]
		size := startingHandSize(k)
		player.Hand = append([]Card(nil), g.Deck[:size]...)
		g.Deck = g.Deck[size:]
	}

	g.Turn = 1
	g.Round++
	g.DrawChoice = nil
	g.Log = nil
	g.Pending = nil
	g.Collective = nil
	g.Bushido = nil
	g.logf("important", "La partie commence. %s (Shogun) joue en premier.", players[shogunIdx].Name)
	g.beginTurn(players[shogunIdx])
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

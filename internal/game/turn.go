package game

import (
	"errors"
	"sort"
)

// Phase est une étape du tour d'un joueur.
type Phase string

const (
	PhaseRecover Phase = "recover"
	PhaseDraw    Phase = "draw"
	PhasePlay    Phase = "play"
	PhaseDiscard Phase = "discard"
)

// MaxHandSize est le nombre maximal de cartes en main à la fin d'un tour.
const MaxHandSize = 7

var (
	ErrNotStarted     = errors.New("game not started")
	ErrNotYourTurn    = errors.New("not your turn")
	ErrWrongPhase     = errors.New("action not allowed in this phase")
	ErrInvalidDiscard = errors.New("invalid discard selection")
	ErrNotInLobby     = errors.New("game is not waiting for players")
)

// sortedPlayers retourne les joueurs dans l'ordre des places (sens horaire).
func (g *Game) sortedPlayers() []*Player {
	players := make([]*Player, 0, len(g.Players))
	for _, p := range g.Players {
		players = append(players, p)
	}
	sort.Slice(players, func(i, j int) bool { return players[i].Position < players[j].Position })
	return players
}

// refresh recalcule les champs publics dérivés (compteurs, statut à terre, piles).
func (g *Game) refresh() {
	for _, p := range g.Players {
		p.HandCount = len(p.Hand)
		switch {
		case p.Character == nil:
			p.Down = ""
		case p.Life <= 0:
			p.Down = "hp"
		case p.HandCount == 0:
			p.Down = "hand"
		default:
			p.Down = ""
		}
	}
	g.syncPiles()
}

// beginTurn résout la récupération (et le Code du Bushido), puis la pioche et ouvre la phase de jeu.
func (g *Game) beginTurn(p *Player) {
	g.Active = p.Name
	g.Played = 0

	g.Phase = PhaseRecover
	if p.Life <= 0 {
		p.Life = p.MaxLife
		g.logf("info", "%s récupère tous ses PV.", p.Name)
	}
	if g.bushidoStep(p) {
		g.refresh()
		return
	}
	g.drawPhase(p)
}

func (g *Game) drawPhase(p *Player) {
	g.Phase = PhaseDraw
	n := 2
	if p.Character != nil && p.Character.ID == "hideyoshi" {
		n++
	}
	if len(g.Players) == 3 && p.Role != nil && p.Role.ID == RoleShogun {
		n++
	}

	// Ieyasu peut remplacer sa première carte par celle du dessus de la défausse : décision du joueur.
	if p.characterID() == "ieyasu" && len(g.Discard) > 0 && !isBotName(p.Name) && g.State == GameStateStarted {
		g.DrawChoice = &DrawChoice{Player: p.Name, Top: g.Discard[len(g.Discard)-1], Count: n}
		g.refresh()
		return
	}
	g.finishDraw(p, n, false)
}

// finishDraw résout la pioche (verrou déjà pris) puis ouvre la phase de jeu.
func (g *Game) finishDraw(p *Player, n int, takeDiscard bool) {
	if takeDiscard && len(g.Discard) > 0 {
		top := g.Discard[len(g.Discard)-1]
		g.Discard = g.Discard[:len(g.Discard)-1]
		p.Hand = append(p.Hand, top)
		g.drawCards(p, n-1)
		g.logf("info", "Tour %d : %s prend %s dans la défausse et pioche %d carte(s).", g.Turn, p.Name, top.Name, n-1)
	} else {
		g.drawCards(p, n)
		g.logf("info", "Tour %d : %s pioche %d cartes.", g.Turn, p.Name, n)
	}

	if g.State == GameStateStarted {
		g.Phase = PhasePlay
	}
	g.refresh()
}

func (g *Game) drawCards(p *Player, n int) {
	for i := 0; i < n; i++ {
		if len(g.Deck) == 0 && !g.reshuffle() {
			return
		}
		p.Hand = append(p.Hand, g.Deck[0])
		g.Deck = g.Deck[1:]
	}
}

// reshuffle reconstitue la pioche avec la défausse ; chaque joueur perd 1 point d'honneur.
func (g *Game) reshuffle() bool {
	if len(g.Discard) == 0 {
		return false
	}
	g.Deck = g.Discard
	g.Discard = nil
	shuffleCards(g.Deck)
	g.logf("important", "La pioche est épuisée : la défausse est remmélangée et chaque joueur perd 1 point d'honneur.")

	for _, p := range g.Players {
		p.Honor--
		if p.Honor <= 0 && g.State == GameStateStarted {
			g.finish("Un joueur n'a plus de point d'honneur (la pioche a été renouvelée).")
		}
	}
	return g.State == GameStateStarted
}

// finish termine la partie et révèle tous les rôles.
func (g *Game) finish(reason string) {
	g.State = GameStateEnded
	g.EndedReason = reason
	g.logf("important", "Partie terminée : %s", reason)
	for _, p := range g.Players {
		p.PublicRole = p.Role
	}
	g.computeResults(reason)
}

func (g *Game) nextTurn() {
	players := g.sortedPlayers()
	for i, p := range players {
		if p.Name == g.Active {
			g.Turn++
			g.beginTurn(players[(i+1)%len(players)])
			return
		}
	}
}

func (g *Game) checkTurn(name string, phase Phase) error {
	switch {
	case g.State != GameStateStarted:
		return ErrNotStarted
	case g.Active != name:
		return ErrNotYourTurn
	case g.Phase != phase:
		return ErrWrongPhase
	}
	return nil
}

// EndTurn termine la phase de jeu ; une main de plus de 7 cartes impose d'abord la défausse.
func (g *Game) EndTurn(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if err := g.checkTurn(name, PhasePlay); err != nil {
		return err
	}
	if g.busy() {
		return ErrAttackPending
	}
	if len(g.Players[name].Hand) > MaxHandSize {
		g.Phase = PhaseDiscard
		g.logf("info", "%s doit défausser avant de terminer son tour.", name)
		return nil
	}
	g.nextTurn()
	return nil
}

// DiscardCards défausse les cartes (indices dans la main) exigées par la phase de défausse, puis passe le tour.
func (g *Game) DiscardCards(name string, indices []int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if err := g.checkTurn(name, PhaseDiscard); err != nil {
		return err
	}
	p := g.Players[name]
	need := len(p.Hand) - MaxHandSize
	chosen := make(map[int]bool, len(indices))
	for _, i := range indices {
		if i < 0 || i >= len(p.Hand) || chosen[i] {
			return ErrInvalidDiscard
		}
		chosen[i] = true
	}
	if len(chosen) != need {
		return ErrInvalidDiscard
	}

	kept := make([]Card, 0, MaxHandSize)
	for i, c := range p.Hand {
		if chosen[i] {
			g.Discard = append(g.Discard, c)
		} else {
			kept = append(kept, c)
		}
	}
	p.Hand = kept
	g.nextTurn()
	return nil
}

// SetReady change l'état "prêt" d'un joueur du lobby.
func (g *Game) SetReady(name string, ready bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State != GameStateWaiting {
		return ErrNotInLobby
	}
	if p, ok := g.Players[name]; ok {
		p.Ready = ready
	}
	return nil
}

// AllReady indique si tous les joueurs, hôte exclu, sont prêts.
func (g *Game) AllReady(host string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for name, p := range g.Players {
		if name != host && !p.Ready {
			return false
		}
	}
	return true
}

package game

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAttackPending = errors.New("a resolution is waiting for a response")
	ErrNoAttack      = errors.New("nothing to respond to")
	ErrNotTarget     = errors.New("you are not the one who must respond")
	ErrCardNotFound  = errors.New("card not found in hand")
	ErrNotPlayable   = errors.New("this card cannot be played now")
	ErrInvalidTarget = errors.New("invalid target or option")
	ErrInvalidParry  = errors.New("this card cannot be used to respond")
)

// LogEntry est un événement public du journal de la partie.
type LogEntry struct {
	Kind string `json:"kind"` // info, action ou important
	Text string `json:"text"`
}

const maxLogEntries = 80

func (g *Game) logf(kind, format string, args ...any) {
	g.Log = append(g.Log, LogEntry{Kind: kind, Text: fmt.Sprintf(format, args...)})
	if len(g.Log) > maxLogEntries {
		g.Log = g.Log[len(g.Log)-maxLogEntries:]
	}
}

// PrivateState regroupe ce que seul le joueur concerné peut voir.
type PrivateState struct {
	Hand          []Card     `json:"hand"`
	Role          *Role      `json:"role"`
	Plays         []Play     `json:"plays"`
	ResponseCards []int      `json:"response_cards"` // cartes utilisables pour la réponse qu'on attend de moi
	WeaponLimit   *Calc      `json:"weapon_limit"`
	Power         *PowerInfo `json:"power"`
}

func (g *Game) PrivateState(name string) PrivateState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	state := PrivateState{Hand: []Card{}, Plays: []Play{}, ResponseCards: []int{}}
	p, ok := g.Players[name]
	if !ok {
		return state
	}
	state.Hand = append(state.Hand, p.Hand...)
	state.Role = p.Role
	if g.State != GameStateStarted || p.Character == nil {
		return state
	}
	for _, c := range p.Hand {
		state.Plays = append(state.Plays, g.playability(p, c))
	}
	switch {
	case g.Pending != nil && g.Pending.Target == name:
		state.ResponseCards = g.parryOptions(p)
	case g.collectivePending(name):
		state.ResponseCards = g.collectiveOptions(g.Collective.Card, p)
	case g.Bushido != nil && g.Bushido.Player == name:
		state.ResponseCards = weaponIndices(p)
	}
	if state.ResponseCards == nil {
		state.ResponseCards = []int{}
	}
	limit := g.weaponLimit(p)
	state.WeaponLimit = &limit
	state.Power = g.powerOf(p)
	return state
}

// TargetsFor liste les cibles possibles de la carte située à l'indice donné de la main du joueur.
func (g *Game) TargetsFor(name string, cardIndex int) ([]Target, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	p, ok := g.Players[name]
	if !ok || cardIndex < 0 || cardIndex >= len(p.Hand) {
		return nil, ErrCardNotFound
	}
	card := p.Hand[cardIndex]
	if !g.playability(p, card).NeedsTarget {
		return nil, ErrNotPlayable
	}
	return g.cardTargets(p, card), nil
}

// declareAttack retire l'arme de la main, annonce l'attaque et la résout si la cible ne peut pas parer.
func (g *Game) declareAttack(att *Player, idx int, t Target) {
	weapon := att.Hand[idx]
	att.Hand = append(att.Hand[:idx], att.Hand[idx+1:]...)
	g.Discard = append(g.Discard, weapon)
	g.Played++
	g.Pending = &Attack{Attacker: att.Name, Target: t.Name, Weapon: weapon, Difficulty: *t.Difficulty, Damage: *t.Damage}
	g.logf("action", "%s attaque %s avec %s (dégâts potentiels : %d).", att.Name, t.Name, weapon.Name, t.Damage.Total)

	if len(g.parryOptions(g.Players[t.Name])) == 0 {
		g.logf("info", "%s n'a aucune parade utilisable.", t.Name)
		g.resolveAttack()
	}
}

// PlayCard joue la carte à l'indice donné ; `target` et `option` précisent la cible et le choix éventuel.
func (g *Game) PlayCard(name string, cardIndex int, targetName, option string) error {
	return g.PlayCardExpect(name, cardIndex, "", targetName, option)
}

// PlayCardExpect comme PlayCard, mais refuse si la carte à cet indice n'est plus `wantID` (requête périmée ou doublon).
func (g *Game) PlayCardExpect(name string, cardIndex int, wantID, targetName, option string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State != GameStateStarted {
		return ErrNotStarted
	}
	p := g.Players[name]
	if p == nil || cardIndex < 0 || cardIndex >= len(p.Hand) || (wantID != "" && p.Hand[cardIndex].ID != wantID) {
		return ErrCardNotFound
	}
	if g.busy() {
		return ErrAttackPending
	}
	if g.Active != name {
		return ErrNotYourTurn
	}
	card := p.Hand[cardIndex]
	pl := g.playability(p, card)
	if !pl.OK {
		if g.Phase != PhasePlay {
			return ErrWrongPhase
		}
		return ErrNotPlayable
	}

	var chosen *Target
	if pl.NeedsTarget {
		for _, t := range g.cardTargets(p, card) {
			if t.Name == targetName {
				t := t
				chosen = &t
			}
		}
		if chosen == nil || !chosen.OK {
			return ErrInvalidTarget
		}
	}

	if card.Category == "arme" {
		g.declareAttack(p, cardIndex, *chosen)
	} else if err := g.applyEffect(p, cardIndex, chosen, option); err != nil {
		return err
	}
	g.refresh()
	return nil
}

// Respond permet de répondre à ce qu'on attend de soi : attaque, Cri de guerre / Ju Jitsu ou Code du Bushido.
// `useCard` désigne la carte à défausser (parade, arme...), sinon on subit la conséquence.
func (g *Game) Respond(name string, useCard bool, cardIndex int) error {
	return g.RespondExpect(name, useCard, cardIndex, "")
}

// RespondExpect comme Respond, mais refuse si la carte défaussée n'est plus `wantID`.
func (g *Game) RespondExpect(name string, useCard bool, cardIndex int, wantID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if p := g.Players[name]; wantID != "" && useCard && p != nil && (cardIndex < 0 || cardIndex >= len(p.Hand) || p.Hand[cardIndex].ID != wantID) {
		return ErrCardNotFound
	}
	return g.respond(name, useCard, cardIndex)
}

func (g *Game) respond(name string, useCard bool, cardIndex int) error {
	switch {
	case g.DrawChoice != nil:
		return g.respondDraw(name, useCard)
	case g.Collective != nil:
		return g.respondCollective(name, useCard, cardIndex)
	case g.Bushido != nil:
		return g.respondBushido(name, useCard, cardIndex)
	case g.Pending == nil:
		return ErrNoAttack
	case g.Pending.Target != name:
		return ErrNotTarget
	}

	tgt := g.Players[name]
	if useCard {
		valid := false
		for _, i := range g.parryOptions(tgt) {
			valid = valid || i == cardIndex
		}
		if !valid {
			return ErrInvalidParry
		}
		card := tgt.Hand[cardIndex]
		tgt.Hand = append(tgt.Hand[:cardIndex], tgt.Hand[cardIndex+1:]...)
		g.Discard = append(g.Discard, card)
		g.logf("action", "%s pare l'attaque de %s avec %s : aucun dégât.", name, g.Pending.Attacker, card.Name)
		g.Pending = nil
	} else {
		g.resolveAttack()
	}
	g.refresh()
	return nil
}

// hurt retire des PV à un joueur ; à 0 PV il perd 1 point d'honneur au profit de `att`.
func (g *Game) hurt(att, tgt *Player, amount int) int {
	if g.State != GameStateStarted {
		return 0
	}
	lost := amount
	if lost > tgt.Life {
		lost = tgt.Life
	}
	tgt.Life -= lost
	g.logf("action", "%s subit %d dégât(s) et passe à %d/%d PV.", tgt.Name, lost, tgt.Life, tgt.MaxLife)

	if lost > 0 && tgt.Life == 0 {
		tgt.Honor--
		att.Honor++
		g.logf("important", "%s est vaincu·e : perd 1 point d'honneur, %s en gagne 1.", tgt.Name, att.Name)
		g.lastDefeat = &defeat{Victim: tgt, Killer: att}
		g.checkEnd()
		g.lastDefeat = nil
	}
	return lost
}

// checkHonor termine la partie dès qu'un joueur n'a plus de point d'honneur.
func (g *Game) checkHonor() {
	for _, p := range g.sortedPlayers() {
		if p.Honor <= 0 && g.State == GameStateStarted {
			g.finish(fmt.Sprintf("%s a atteint 0 point d'honneur.", p.Name))
		}
	}
}

// resolveAttack applique les dégâts de l'attaque en attente (verrou déjà pris).
func (g *Game) resolveAttack() {
	a := g.Pending
	g.Pending = nil
	att, tgt := g.Players[a.Attacker], g.Players[a.Target]

	lost := g.hurt(att, tgt, a.Damage.Total)
	if lost > 0 {
		switch att.characterID() {
		case "tomoe":
			g.drawCards(att, 1)
			g.logf("info", "Tomoe pioche 1 carte (son arme a blessé un joueur).")
		case "ushiwaka":
			g.drawCards(att, lost)
			g.logf("info", "Ushiwaka pioche %d carte(s) (PV perdus à cause de son arme).", lost)
		}
	}
}

// NeedsBotStep indique si un joueur automatique (bot ou joueur déconnecté) doit agir.
func (g *Game) NeedsBotStep() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.State != GameStateStarted {
		return false
	}
	switch {
	case g.DrawChoice != nil:
		return g.automated(g.DrawChoice.Player)
	case g.Pending != nil:
		return g.automated(g.Pending.Target)
	case g.Collective != nil:
		for _, r := range g.Collective.Rows {
			if r.Status == "pending" && g.automated(r.Name) {
				return true
			}
		}
		return false
	case g.Bushido != nil:
		return g.automated(g.Bushido.Player)
	}
	return g.automated(g.Active)
}

func isBotName(name string) bool { return strings.HasPrefix(name, BotPrefix) }

// automated indique si le serveur joue pour ce joueur : bot, ou humain absent.
func (g *Game) automated(name string) bool {
	p := g.Players[name]
	return p != nil && (isBotName(name) || p.Away)
}

// SetAway (dé)marque l'automatisation d'un joueur déconnecté ; retourne true si l'état a changé.
func (g *Game) SetAway(name string, away bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	p := g.Players[name]
	if p == nil || isBotName(name) || p.Away == away || g.State == GameStateWaiting {
		return false
	}
	p.Away = away
	if away {
		g.logf("important", "%s est déconnecté·e : le serveur répond et passe son tour à sa place.", name)
	} else {
		g.logf("important", "%s est de retour et reprend la main.", name)
	}
	return true
}

// AllHumansAway indique qu'aucun joueur humain n'est présent dans une partie commencée ou terminée.
func (g *Game) AllHumansAway() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.State == GameStateWaiting {
		return false
	}
	for name, p := range g.Players {
		if !isBotName(name) && !p.Away {
			return false
		}
	}
	return true
}

// PlayBotStep fait agir un bot : réponse, attaque d'un joueur, ou fin de tour. Retourne l'acteur.
func (g *Game) PlayBotStep() (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State != GameStateStarted {
		return "", false
	}

	switch {
	case g.DrawChoice != nil:
		name := g.DrawChoice.Player
		if !g.automated(name) {
			return "", false
		}
		_ = g.respondDraw(name, false)
		g.refresh()
		return name, true
	case g.Pending != nil:
		bot := g.Players[g.Pending.Target]
		if !g.automated(bot.Name) {
			return "", false
		}
		options := g.parryOptions(bot)
		g.botRespond(bot.Name, options)
		return bot.Name, true
	case g.Collective != nil:
		for _, r := range g.Collective.Rows {
			if r.Status == "pending" && g.automated(r.Name) {
				g.botRespond(r.Name, g.collectiveOptions(g.Collective.Card, g.Players[r.Name]))
				return r.Name, true
			}
		}
		return "", false
	case g.Bushido != nil:
		bot := g.Players[g.Bushido.Player]
		if !g.automated(bot.Name) {
			return "", false
		}
		g.botRespond(bot.Name, weaponIndices(bot))
		return bot.Name, true
	}

	if !g.automated(g.Active) {
		return "", false
	}
	bot := g.Players[g.Active]

	// Un bot attaque une fois par tour (un joueur absent ne fait que passer) : première arme jouable sur la première cible valide, joueurs réels d'abord.
	if isBotName(bot.Name) && g.Phase == PhasePlay && g.Played == 0 {
		for i, c := range bot.Hand {
			if c.Category != "arme" || !g.playability(bot, c).OK {
				continue
			}
			var pick *Target
			for _, t := range g.weaponTargets(bot, c) {
				t := t
				if t.OK && (pick == nil || (isBotName(pick.Name) && !isBotName(t.Name))) {
					pick = &t
				}
			}
			if pick == nil {
				continue
			}
			g.declareAttack(bot, i, *pick)
			g.refresh()
			return bot.Name, true
		}
	}

	if extra := len(bot.Hand) - MaxHandSize; extra > 0 {
		g.Discard = append(g.Discard, bot.Hand[:extra]...)
		bot.Hand = bot.Hand[extra:]
	}
	g.nextTurn()
	return bot.Name, true
}

// botRespond répond pour un bot : première carte valide si elle existe, sinon on subit (verrou déjà pris).
func (g *Game) botRespond(name string, options []int) {
	if len(options) > 0 {
		_ = g.respond(name, true, options[0])
		return
	}
	_ = g.respond(name, false, 0)
}

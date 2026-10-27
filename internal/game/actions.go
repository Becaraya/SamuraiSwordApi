package game

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAttackPending = errors.New("an attack is waiting for a response")
	ErrNoAttack      = errors.New("no attack to respond to")
	ErrNotTarget     = errors.New("you are not the target of this attack")
	ErrCardNotFound  = errors.New("card not found in hand")
	ErrNotPlayable   = errors.New("this card cannot be played now")
	ErrInvalidTarget = errors.New("invalid target")
	ErrInvalidParry  = errors.New("this card cannot be used as a parry")
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
	Hand        []Card `json:"hand"`
	Role        *Role  `json:"role"`
	Plays       []Play `json:"plays"`
	ParryCards  []int  `json:"parry_cards"`
	WeaponLimit *Calc  `json:"weapon_limit"`
}

func (g *Game) PrivateState(name string) PrivateState {
	g.mu.RLock()
	defer g.mu.RUnlock()

	state := PrivateState{Hand: []Card{}, Plays: []Play{}, ParryCards: []int{}}
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
	if g.Pending != nil && g.Pending.Target == name {
		state.ParryCards = g.parryOptions(p)
	}
	limit := g.weaponLimit(p)
	state.WeaponLimit = &limit
	return state
}

// TargetsFor liste les cibles possibles de l'arme située à l'indice donné de la main du joueur.
func (g *Game) TargetsFor(name string, cardIndex int) ([]Target, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	p, ok := g.Players[name]
	if !ok || cardIndex < 0 || cardIndex >= len(p.Hand) {
		return nil, ErrCardNotFound
	}
	if p.Hand[cardIndex].Category != "arme" {
		return nil, ErrNotPlayable
	}
	return g.weaponTargets(p, p.Hand[cardIndex]), nil
}

// PlayWeapon annonce une attaque. Sans parade possible, elle est résolue immédiatement.
func (g *Game) PlayWeapon(name string, cardIndex int, targetName string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State != GameStateStarted {
		return ErrNotStarted
	}
	att := g.Players[name]
	if att == nil || cardIndex < 0 || cardIndex >= len(att.Hand) {
		return ErrCardNotFound
	}
	if g.Pending != nil {
		return ErrAttackPending
	}
	if g.Active != name {
		return ErrNotYourTurn
	}
	weapon := att.Hand[cardIndex]
	if p := g.playability(att, weapon); !p.OK {
		if g.Phase != PhasePlay {
			return ErrWrongPhase
		}
		return ErrNotPlayable
	}

	tgt := g.Players[targetName]
	if tgt == nil || tgt == att {
		return ErrInvalidTarget
	}
	var chosen *Target
	for _, t := range g.weaponTargets(att, weapon) {
		if t.Name == targetName {
			t := t
			chosen = &t
		}
	}
	if chosen == nil || !chosen.OK {
		return ErrInvalidTarget
	}

	att.Hand = append(att.Hand[:cardIndex], att.Hand[cardIndex+1:]...)
	g.Discard = append(g.Discard, weapon)
	g.Played++
	g.Pending = &Attack{Attacker: name, Target: targetName, Weapon: weapon, Difficulty: *chosen.Difficulty, Damage: *chosen.Damage}
	g.logf("action", "%s attaque %s avec %s (dégâts potentiels : %d).", name, targetName, weapon.Name, chosen.Damage.Total)

	if len(g.parryOptions(tgt)) == 0 {
		g.logf("info", "%s n'a aucune parade utilisable.", targetName)
		g.resolveAttack()
	}
	g.refresh()
	return nil
}

// Respond permet à la cible de parer (carte de la main) ou de subir l'attaque.
func (g *Game) Respond(name string, parry bool, cardIndex int) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Pending == nil {
		return ErrNoAttack
	}
	if g.Pending.Target != name {
		return ErrNotTarget
	}
	tgt := g.Players[name]
	if parry {
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

// resolveAttack applique les dégâts de l'attaque en attente (verrou déjà pris).
func (g *Game) resolveAttack() {
	a := g.Pending
	g.Pending = nil
	att, tgt := g.Players[a.Attacker], g.Players[a.Target]

	lost := a.Damage.Total
	if lost > tgt.Life {
		lost = tgt.Life
	}
	tgt.Life -= lost
	g.logf("action", "%s subit %d dégât(s) et passe à %d/%d PV.", tgt.Name, lost, tgt.Life, tgt.MaxLife)

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

	if tgt.Life == 0 {
		tgt.Honor--
		att.Honor++
		g.logf("important", "%s est vaincu·e : perd 1 point d'honneur, %s en gagne 1.", tgt.Name, att.Name)
		for _, p := range g.sortedPlayers() {
			if p.Honor <= 0 && g.State == GameStateStarted {
				g.finish(fmt.Sprintf("%s a atteint 0 point d'honneur.", p.Name))
			}
		}
	}
}

// NeedsBotStep indique si un bot doit agir (tour de bot ou réponse d'un bot à une attaque).
func (g *Game) NeedsBotStep() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.State != GameStateStarted {
		return false
	}
	if g.Pending != nil {
		return strings.HasPrefix(g.Pending.Target, BotPrefix)
	}
	return strings.HasPrefix(g.Active, BotPrefix)
}

// PlayBotStep fait agir un bot : réponse à une attaque, attaque d'un joueur, ou fin de tour. Retourne l'acteur.
func (g *Game) PlayBotStep() (string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.State != GameStateStarted {
		return "", false
	}

	if g.Pending != nil {
		bot := g.Players[g.Pending.Target]
		if !strings.HasPrefix(bot.Name, BotPrefix) {
			return "", false
		}
		if options := g.parryOptions(bot); len(options) > 0 {
			card := bot.Hand[options[0]]
			bot.Hand = append(bot.Hand[:options[0]], bot.Hand[options[0]+1:]...)
			g.Discard = append(g.Discard, card)
			g.logf("action", "%s pare l'attaque de %s avec %s : aucun dégât.", bot.Name, g.Pending.Attacker, card.Name)
			g.Pending = nil
		} else {
			g.resolveAttack()
		}
		g.refresh()
		return bot.Name, true
	}

	if !strings.HasPrefix(g.Active, BotPrefix) {
		return "", false
	}
	bot := g.Players[g.Active]

	// Un bot attaque une fois par tour : la première arme jouable sur la première cible valide (joueurs réels d'abord).
	if g.Phase == PhasePlay && g.Played == 0 {
		for i, c := range bot.Hand {
			if c.Category != "arme" || !g.playability(bot, c).OK {
				continue
			}
			var pick *Target
			for _, t := range g.weaponTargets(bot, c) {
				t := t
				if t.OK && (pick == nil || (strings.HasPrefix(pick.Name, BotPrefix) && !strings.HasPrefix(t.Name, BotPrefix))) {
					pick = &t
				}
			}
			if pick == nil {
				continue
			}
			tgt := g.Players[pick.Name]
			bot.Hand = append(bot.Hand[:i], bot.Hand[i+1:]...)
			g.Discard = append(g.Discard, c)
			g.Played++
			g.Pending = &Attack{Attacker: bot.Name, Target: tgt.Name, Weapon: c, Difficulty: *pick.Difficulty, Damage: *pick.Damage}
			g.logf("action", "%s attaque %s avec %s (dégâts potentiels : %d).", bot.Name, tgt.Name, c.Name, pick.Damage.Total)
			if len(g.parryOptions(tgt)) == 0 {
				g.logf("info", "%s n'a aucune parade utilisable.", tgt.Name)
				g.resolveAttack()
			}
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

package game

import (
	"fmt"
	"math/rand"
)

// CollectiveRow est l'état de réponse d'un joueur à un effet collectif (Cri de guerre, Ju Jitsu).
type CollectiveRow struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pending, done ou exempt
	Text   string `json:"text,omitempty"`
}

// Collective est un effet qui demande une réponse à plusieurs joueurs.
type Collective struct {
	Card   Card            `json:"card"`
	Issuer string          `json:"issuer"`
	Rows   []CollectiveRow `json:"rows"`
}

// BushidoChoice est la décision à prendre quand le Code du Bushido révèle une arme.
type BushidoChoice struct {
	Player   string `json:"player"`
	Revealed Card   `json:"revealed"`
	Next     string `json:"next"` // voisin de gauche, qui recevra le Code
	Free     bool   `json:"free"` // le Shogun à 3 joueurs ne perd pas d'honneur
}

const codeBushido = "code_du_bushido"

func (g *Game) busy() bool {
	return g.Pending != nil || g.Collective != nil || g.Bushido != nil || g.DrawChoice != nil
}

// DrawChoice est la décision d'Ieyasu en phase de pioche : carte de la défausse ou pioche normale.
type DrawChoice struct {
	Player string `json:"player"`
	Top    Card   `json:"top"`
	Count  int    `json:"count"`
}

// PowerInfo décrit un pouvoir de personnage à activer volontairement.
type PowerInfo struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// powerOf retourne le pouvoir activable du joueur (Nobunaga : sacrifier 1 PV, sauf le dernier, pour piocher).
func (g *Game) powerOf(p *Player) *PowerInfo {
	if p.characterID() != "nobunaga" || g.State != GameStateStarted {
		return nil
	}
	info := &PowerInfo{ID: "nobunaga", Label: "Sacrifier 1 PV : piocher 1 carte"}
	switch {
	case g.busy():
		info.Reason = "Une résolution est en cours."
	case g.Active != p.Name || g.Phase != PhasePlay:
		info.Reason = "Disponible pendant votre phase de jeu."
	case p.Life <= 1:
		info.Reason = "Vous ne pouvez pas sacrifier votre dernier PV."
	default:
		info.OK = true
	}
	return info
}

// UsePower active le pouvoir volontaire du personnage du joueur.
func (g *Game) UsePower(name, power string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	p := g.Players[name]
	if g.State != GameStateStarted || p == nil {
		return ErrNotStarted
	}
	info := g.powerOf(p)
	if info == nil || info.ID != power {
		return ErrNotPlayable
	}
	if !info.OK {
		if g.busy() {
			return ErrAttackPending
		}
		if g.Active != name {
			return ErrNotYourTurn
		}
		if g.Phase != PhasePlay {
			return ErrWrongPhase
		}
		return ErrNotPlayable
	}

	p.Life--
	g.logf("action", "Nobunaga (%s) sacrifie 1 PV (%d/%d) et pioche 1 carte.", p.Name, p.Life, p.MaxLife)
	g.drawCards(p, 1)
	g.refresh()
	return nil
}

func (g *Game) respondDraw(name string, takeDiscard bool) error {
	if g.DrawChoice.Player != name {
		return ErrNotTarget
	}
	p := g.Players[name]
	n := g.DrawChoice.Count
	g.DrawChoice = nil
	g.finishDraw(p, n, takeDiscard)
	return nil
}

func weaponIndices(p *Player) []int {
	var idx []int
	for i, c := range p.Hand {
		if c.Category == "arme" {
			idx = append(idx, i)
		}
	}
	return idx
}

func (g *Game) collectiveOptions(card Card, p *Player) []int {
	if card.ID == "ju_jitsu" {
		return weaponIndices(p)
	}
	return g.parryOptions(p)
}

func (g *Game) collectivePending(name string) bool {
	if g.Collective == nil {
		return false
	}
	for _, r := range g.Collective.Rows {
		if r.Name == name && r.Status == "pending" {
			return true
		}
	}
	return false
}

// collectiveExemption retourne la raison pour laquelle un joueur n'est pas affecté, ou "".
func collectiveExemption(p *Player) string {
	switch {
	case p.isDown():
		return fmt.Sprintf("%s : non affecté", p.downLabel())
	case p.characterID() == "chiyome":
		return "Chiyome : seules les armes la blessent"
	}
	return ""
}

func (g *Game) codeHolder() *Player {
	for _, p := range g.sortedPlayers() {
		if countCards(p.Permanents, codeBushido) > 0 {
			return p
		}
	}
	return nil
}

func (g *Game) neighbourOf(p *Player) *Player {
	players := g.sortedPlayers()
	for i, q := range players {
		if q == p {
			return players[(i+1)%len(players)]
		}
	}
	return p
}

// cardTargets liste les cibles d'une carte qui en demande une.
func (g *Game) cardTargets(p *Player, c Card) []Target {
	if c.Category == "arme" {
		return g.weaponTargets(p, c)
	}
	var targets []Target
	for _, q := range g.sortedPlayers() {
		t := Target{Name: q.Name}
		switch c.ID {
		case "diversion":
			if q == p {
				continue
			}
			t.OK = len(q.Hand) > 0
			t.Short = fmt.Sprintf("%d carte(s)", len(q.Hand))
			if !t.OK {
				t.Reason = fmt.Sprintf("%s n'a aucune carte en main.", q.Name)
			}
		case "geisha":
			if q == p {
				continue
			}
			for i, perm := range q.Permanents {
				t.Options = append(t.Options, Option{ID: fmt.Sprintf("perm:%d", i), Label: "Défausser " + perm.Name + " (permanente)"})
			}
			if len(q.Hand) > 0 {
				t.Options = append(t.Options, Option{ID: "hand", Label: fmt.Sprintf("Une carte au hasard dans sa main (%d carte(s))", len(q.Hand))})
			}
			t.OK = len(t.Options) > 0
			t.Short = "Cible possible"
			if !t.OK {
				t.Short = "Rien à défausser"
				t.Reason = fmt.Sprintf("%s n'a ni permanente ni carte en main : Geisha n'a aucun effet.", q.Name)
			}
		case "meditation":
			if q == p {
				continue
			}
			t.OK = true
			t.Short = "Piochera 1 carte"
		case codeBushido:
			t.OK = true
			t.Short = "Placer le Code ici"
		}
		targets = append(targets, t)
	}
	return targets
}

// playability évalue une carte de la main du joueur dans l'état courant.
func (g *Game) playability(p *Player, c Card) Play {
	switch {
	case g.State != GameStateStarted:
		return Play{Reason: "La partie n'est pas en cours."}
	case g.busy():
		return Play{Reason: "Une résolution est en cours (attaque, effet collectif ou Code du Bushido)."}
	case c.ID == "parade":
		return Play{Reason: "Une Parade se joue en réaction à une attaque qui vous cible, même hors de votre tour."}
	case g.Active != p.Name:
		return Play{Reason: "Cette carte n'est jouable que pendant votre tour."}
	case g.Phase != PhasePlay:
		return Play{Reason: "Les cartes se jouent pendant la phase de jeu."}
	}

	anyTarget := func(reason string) Play {
		for _, t := range g.cardTargets(p, c) {
			if t.OK {
				return Play{OK: true, NeedsTarget: true}
			}
		}
		return Play{Reason: reason}
	}

	switch c.ID {
	case "diversion", "geisha", "meditation":
		return anyTarget("Aucune cible valide pour cette carte.")
	case codeBushido:
		if holder := g.codeHolder(); holder != nil {
			return Play{Reason: fmt.Sprintf("Un seul Code du Bushido peut être en jeu à la fois (devant %s).", holder.Name)}
		}
		return anyTarget("Aucune cible valide.")
	case "cri_de_guerre", "ju_jitsu":
		for _, q := range g.Players {
			if q != p && collectiveExemption(q) == "" {
				return Play{OK: true}
			}
		}
		return Play{Reason: "Aucun joueur n'est affecté par cet effet."}
	case "daimyo", "ceremonie_du_the", "armure", "attaque_rapide", "concentration":
		return Play{OK: true}
	}

	if c.Category == "arme" {
		if limit := g.weaponLimit(p); g.Played >= limit.Total {
			return Play{Reason: fmt.Sprintf("Vous avez déjà joué le maximum d'armes ce tour (%d/%d).", g.Played, limit.Total)}
		}
		return anyTarget("Aucune cible valide : toutes les cibles sont à terre ou hors de portée.")
	}
	return Play{Reason: "L'effet de cette carte n'est pas encore disponible."}
}

func (g *Game) discardFromHand(p *Player, idx int) Card {
	c := p.Hand[idx]
	p.Hand = append(p.Hand[:idx], p.Hand[idx+1:]...)
	g.Discard = append(g.Discard, c)
	return c
}

// applyEffect résout une carte non-arme (verrou déjà pris, jouabilité déjà vérifiée).
func (g *Game) applyEffect(p *Player, idx int, target *Target, option string) error {
	card := p.Hand[idx]
	var tgt *Player
	if target != nil {
		tgt = g.Players[target.Name]
	}

	switch card.ID {
	case "geisha":
		valid := false
		for _, o := range target.Options {
			valid = valid || o.ID == option
		}
		if !valid {
			return ErrInvalidTarget
		}
		g.discardFromHand(p, idx)
		if option == "hand" {
			i := rand.Intn(len(tgt.Hand))
			dropped := g.discardFromHand(tgt, i)
			g.logf("action", "%s joue Geisha : une carte au hasard de %s est défaussée (%s).", p.Name, tgt.Name, dropped.Name)
		} else {
			var i int
			fmt.Sscanf(option, "perm:%d", &i)
			dropped := tgt.Permanents[i]
			tgt.Permanents = append(tgt.Permanents[:i], tgt.Permanents[i+1:]...)
			g.Discard = append(g.Discard, dropped)
			g.logf("action", "%s joue Geisha : %s est défaussée devant %s.", p.Name, dropped.Name, tgt.Name)
		}
	case "diversion":
		g.discardFromHand(p, idx)
		i := rand.Intn(len(tgt.Hand))
		stolen := tgt.Hand[i]
		tgt.Hand = append(tgt.Hand[:i], tgt.Hand[i+1:]...)
		p.Hand = append(p.Hand, stolen)
		g.logf("action", "%s joue Diversion : prend une carte au hasard dans la main de %s.", p.Name, tgt.Name)
	case "meditation":
		g.discardFromHand(p, idx)
		p.Life = p.MaxLife
		g.logf("action", "%s joue Méditation : récupère tous ses PV (%d/%d).", p.Name, p.Life, p.MaxLife)
		g.drawCards(tgt, 1)
		g.logf("info", "%s pioche 1 carte.", tgt.Name)
	case "daimyo":
		g.discardFromHand(p, idx)
		g.drawCards(p, 2)
		g.logf("action", "%s joue Daimyō et pioche 2 cartes.", p.Name)
	case "ceremonie_du_the":
		g.discardFromHand(p, idx)
		g.drawCards(p, 3)
		for _, q := range g.sortedPlayers() {
			if q != p {
				g.drawCards(q, 1)
			}
		}
		g.logf("action", "%s joue Cérémonie du thé : pioche 3 cartes, les autres joueurs en piochent 1.", p.Name)
	case "armure", "attaque_rapide", "concentration":
		p.Hand = append(p.Hand[:idx], p.Hand[idx+1:]...)
		p.Permanents = append(p.Permanents, card)
		g.logf("action", "%s pose %s devant lui.", p.Name, card.Name)
	case codeBushido:
		p.Hand = append(p.Hand[:idx], p.Hand[idx+1:]...)
		tgt.Permanents = append(tgt.Permanents, card)
		g.logf("action", "%s place le Code du Bushido devant %s.", p.Name, tgt.Name)
	case "cri_de_guerre", "ju_jitsu":
		g.discardFromHand(p, idx)
		g.logf("action", "%s joue %s.", p.Name, card.Name)
		g.startCollective(p, card)
	default:
		return ErrNotPlayable
	}
	return nil
}

func (g *Game) startCollective(issuer *Player, card Card) {
	col := &Collective{Card: card, Issuer: issuer.Name}
	var auto []*Player
	for _, q := range g.sortedPlayers() {
		if q == issuer {
			continue
		}
		if why := collectiveExemption(q); why != "" {
			col.Rows = append(col.Rows, CollectiveRow{Name: q.Name, Status: "exempt", Text: why})
		} else if len(g.collectiveOptions(card, q)) == 0 {
			col.Rows = append(col.Rows, CollectiveRow{Name: q.Name, Status: "done", Text: "perd 1 PV (aucune carte à défausser)"})
			auto = append(auto, q)
		} else {
			col.Rows = append(col.Rows, CollectiveRow{Name: q.Name, Status: "pending"})
		}
	}
	g.Collective = col
	for _, q := range auto {
		g.hurt(issuer, q, 1)
	}
	g.finishCollective()
}

func (g *Game) finishCollective() {
	if g.Collective == nil {
		return
	}
	for _, r := range g.Collective.Rows {
		if r.Status == "pending" {
			return
		}
	}
	g.logf("info", "%s : tous les joueurs concernés ont répondu.", g.Collective.Card.Name)
	g.Collective = nil
}

func (g *Game) respondCollective(name string, useCard bool, idx int) error {
	if !g.collectivePending(name) {
		return ErrNotTarget
	}
	p := g.Players[name]
	issuer := g.Players[g.Collective.Issuer]
	var row *CollectiveRow
	for i := range g.Collective.Rows {
		if g.Collective.Rows[i].Name == name {
			row = &g.Collective.Rows[i]
		}
	}

	if useCard {
		valid := false
		for _, i := range g.collectiveOptions(g.Collective.Card, p) {
			valid = valid || i == idx
		}
		if !valid {
			return ErrInvalidParry
		}
		card := g.discardFromHand(p, idx)
		row.Status, row.Text = "done", "a défaussé "+card.Name
		g.logf("action", "%s défausse %s (%s).", name, card.Name, g.Collective.Card.Name)
	} else {
		row.Status, row.Text = "done", "perd 1 PV"
		g.hurt(issuer, p, 1)
	}
	g.finishCollective()
	g.refresh()
	return nil
}

// ---------- Code du Bushido ----------

func (g *Game) passCode(p *Player) {
	for i, c := range p.Permanents {
		if c.ID == codeBushido {
			p.Permanents = append(p.Permanents[:i], p.Permanents[i+1:]...)
			next := g.neighbourOf(p)
			next.Permanents = append(next.Permanents, c)
			g.logf("action", "Le Code du Bushido passe à %s.", next.Name)
			return
		}
	}
}

// bushidoStep résout le Code du Bushido en fin de récupération ; retourne true si une décision est attendue.
func (g *Game) bushidoStep(p *Player) bool {
	if countCards(p.Permanents, codeBushido) == 0 {
		return false
	}
	if len(g.Deck) == 0 && !g.reshuffle() {
		g.passCode(p)
		return false
	}
	top := g.Deck[0]
	g.Deck = g.Deck[1:]
	g.Discard = append(g.Discard, top)
	g.logf("action", "Code du Bushido : la carte révélée est %s, elle est défaussée.", top.Name)

	if top.Category != "arme" {
		g.passCode(p)
		return false
	}
	free := len(g.Players) == 3 && p.Role != nil && p.Role.ID == RoleShogun
	g.Bushido = &BushidoChoice{Player: p.Name, Revealed: top, Next: g.neighbourOf(p).Name, Free: free}
	return true
}

func (g *Game) respondBushido(name string, discardWeapon bool, idx int) error {
	b := g.Bushido
	if b.Player != name {
		return ErrNotTarget
	}
	p := g.Players[name]
	if discardWeapon {
		valid := false
		for _, i := range weaponIndices(p) {
			valid = valid || i == idx
		}
		if !valid {
			return ErrInvalidParry
		}
		g.discardFromHand(p, idx)
		g.logf("action", "%s défausse une arme pour le Code du Bushido.", name)
		g.passCode(p)
	} else {
		for i, c := range p.Permanents {
			if c.ID == codeBushido {
				p.Permanents = append(p.Permanents[:i], p.Permanents[i+1:]...)
				g.Discard = append(g.Discard, c)
				break
			}
		}
		if b.Free {
			g.logf("important", "%s défausse le Code du Bushido (le Shogun ne perd pas d'honneur à 3 joueurs).", name)
		} else {
			p.Honor--
			g.logf("important", "%s perd 1 point d'honneur et défausse le Code du Bushido.", name)
			g.checkHonor()
		}
	}
	g.Bushido = nil
	if g.State == GameStateStarted {
		g.drawPhase(p)
	}
	g.refresh()
	return nil
}

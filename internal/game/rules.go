package game

import "fmt"

// CalcPart est une ligne du détail d'une valeur calculée (difficulté, dégâts, limite d'armes).
type CalcPart struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// Calc est une valeur calculée avec le détail de ses modificateurs.
type Calc struct {
	Total   int        `json:"total"`
	Parts   []CalcPart `json:"parts"`
	Ignored bool       `json:"ignored,omitempty"`
}

func newCalc(parts ...CalcPart) Calc {
	c := Calc{Parts: parts}
	for _, p := range parts {
		c.Total += p.Value
	}
	return c
}

// Attack est une attaque d'arme annoncée, en attente de la réaction de sa cible.
type Attack struct {
	Attacker   string `json:"attacker"`
	Target     string `json:"target"`
	Weapon     Card   `json:"weapon"`
	Difficulty Calc   `json:"difficulty"`
	Damage     Calc   `json:"damage"`
}

// Option est un choix à faire une fois la cible sélectionnée (ex. Geisha : quelle carte défausser).
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Target décrit une cible possible (ou non) d'une carte.
type Target struct {
	Name       string   `json:"name"`
	OK         bool     `json:"ok"`
	Short      string   `json:"short"`
	Reason     string   `json:"reason,omitempty"`
	Difficulty *Calc    `json:"difficulty,omitempty"`
	Damage     *Calc    `json:"damage,omitempty"`
	Options    []Option `json:"options,omitempty"`
}

// Play indique si une carte de la main est jouable maintenant, et pourquoi sinon.
type Play struct {
	OK          bool   `json:"ok"`
	NeedsTarget bool   `json:"needs_target,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func countCards(cards []Card, id string) int {
	n := 0
	for _, c := range cards {
		if c.ID == id {
			n++
		}
	}
	return n
}

func (p *Player) characterID() string {
	if p.Character == nil {
		return ""
	}
	return p.Character.ID
}

func (p *Player) isDown() bool {
	return p.Life <= 0 || len(p.Hand) == 0
}

func (p *Player) downLabel() string {
	if p.Life <= 0 {
		return "à terre (0 PV)"
	}
	return "à terre (aucune carte en main)"
}

// distance retourne la distance minimale entre deux joueurs : les joueurs à terre ne comptent plus comme intermédiaires.
func (g *Game) distance(att, tgt *Player) (int, bool) {
	var ring []*Player
	for _, p := range g.sortedPlayers() {
		if p == att || !p.isDown() {
			ring = append(ring, p)
		}
	}
	ia, it := -1, -1
	for i, p := range ring {
		if p == att {
			ia = i
		}
		if p == tgt {
			it = i
		}
	}
	if ia < 0 || it < 0 {
		return 0, false
	}
	d := ia - it
	if d < 0 {
		d = -d
	}
	if other := len(ring) - d; other < d {
		d = other
	}
	return d, true
}

func (g *Game) difficulty(att, tgt *Player) (Calc, bool) {
	d, ok := g.distance(att, tgt)
	if !ok {
		return Calc{}, false
	}
	parts := []CalcPart{{Label: "Distance autour de la table", Value: d}}
	if att.characterID() == "kojiro" {
		c := newCalc(parts...)
		c.Ignored = true
		return c, true
	}
	if armor := countCards(tgt.Permanents, "armure"); armor > 0 {
		label := "Armure de " + tgt.Name
		if armor > 1 {
			label += fmt.Sprintf(" ×%d", armor)
		}
		parts = append(parts, CalcPart{Label: label, Value: armor})
	}
	if tgt.characterID() == "benkei" {
		parts = append(parts, CalcPart{Label: "Benkei (capacité)", Value: 1})
	}
	return newCalc(parts...), true
}

func (g *Game) damage(att, tgt *Player, weapon Card) Calc {
	parts := []CalcPart{{Label: weapon.Name, Value: weapon.Damage}}
	if att.characterID() == "musashi" {
		parts = append(parts, CalcPart{Label: "Musashi (capacité)", Value: 1})
	}
	if quick := countCards(att.Permanents, "attaque_rapide"); quick > 0 {
		label := "Attaque rapide"
		if quick > 1 {
			label += fmt.Sprintf(" ×%d", quick)
		}
		parts = append(parts, CalcPart{Label: label, Value: quick})
	}
	if tgt.characterID() == "ginchiyo" {
		parts = append(parts, CalcPart{Label: "Ginchiyo (réduction)", Value: -1})
	}
	c := newCalc(parts...)
	if c.Total < 1 {
		c.Total = 1
	}
	return c
}

func (g *Game) weaponLimit(p *Player) Calc {
	parts := []CalcPart{{Label: "Base", Value: 1}}
	if p.characterID() == "goemon" {
		parts = append(parts, CalcPart{Label: "Goemon (capacité)", Value: 1})
	}
	if conc := countCards(p.Permanents, "concentration"); conc > 0 {
		label := "Concentration"
		if conc > 1 {
			label += fmt.Sprintf(" ×%d", conc)
		}
		parts = append(parts, CalcPart{Label: label, Value: conc})
	}
	if len(g.Players) == 3 && p.Role != nil && p.Role.ID == RoleShogun {
		parts = append(parts, CalcPart{Label: "Shogun à 3 joueurs", Value: 1})
	}
	return newCalc(parts...)
}

// weaponTargets évalue chaque autre joueur comme cible de l'arme.
func (g *Game) weaponTargets(att *Player, weapon Card) []Target {
	var targets []Target
	for _, p := range g.sortedPlayers() {
		if p == att {
			continue
		}
		t := Target{Name: p.Name}
		if p.isDown() {
			t.Short = "À terre"
			t.Reason = fmt.Sprintf("%s est %s : une arme ne peut pas le cibler.", p.Name, p.downLabel())
			targets = append(targets, t)
			continue
		}
		diff, _ := g.difficulty(att, p)
		t.Difficulty = &diff
		if !diff.Ignored && diff.Total > weapon.Precision {
			t.Short = fmt.Sprintf("Difficulté %d > %d", diff.Total, weapon.Precision)
			t.Reason = fmt.Sprintf("Précision insuffisante : difficulté %d, précision de %s %d.", diff.Total, weapon.Name, weapon.Precision)
		} else {
			t.OK = true
			t.Short = fmt.Sprintf("Difficulté %d ≤ %d", diff.Total, weapon.Precision)
			if diff.Ignored {
				t.Short = "Sans difficulté"
			}
			dmg := g.damage(att, p, weapon)
			t.Damage = &dmg
		}
		targets = append(targets, t)
	}
	return targets
}

// parryOptions retourne les indices des cartes de la main utilisables comme parade.
func (g *Game) parryOptions(p *Player) []int {
	var idx []int
	for i, c := range p.Hand {
		hanzoWeapon := p.characterID() == "hanzo" && c.Category == "arme" && len(p.Hand) > 1
		if c.ID == "parade" || hanzoWeapon {
			idx = append(idx, i)
		}
	}
	return idx
}

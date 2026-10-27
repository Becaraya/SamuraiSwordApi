package game

import (
	"fmt"
	"sort"
	"strconv"
)

// Pénalité de Lame perdue (points d'honneur retirés au total du camp de la victime).
const lostBladePenalty = 3

// Types de victoire.
const (
	VictoryHonor = "honneur" // décompte des points d'honneur
	VictorySword = "epee"    // victoire au fil de l'épée
)

// ScoreRow détaille le score d'un joueur en fin de partie.
type ScoreRow struct {
	Name      string `json:"name"`
	Character string `json:"character"`
	Role      Role   `json:"role"`
	Honor     int    `json:"honor"`
	Mult      int    `json:"mult"`
	MultNote  string `json:"mult_note,omitempty"`
	Daimyo    int    `json:"daimyo"`
	Score     int    `json:"score"`
}

// CampScore est le total d'un camp (shogun_samourai, ninja ou ronin), pénalité de Lame perdue incluse.
type CampScore struct {
	Camp    string `json:"camp"`
	Total   int    `json:"total"`
	Penalty int    `json:"penalty,omitempty"`
}

// Results explique le calcul des scores : honneur de base, multiplicateur du rôle, bonus Daimyō, pénalités.
type Results struct {
	Reason  string      `json:"reason"`
	Rows    []ScoreRow  `json:"rows"`
	Camps   []CampScore `json:"camps"` // triés : égalité départagée Ninja > Shogun/Samouraï > Ronin
	Winner  string      `json:"winner"`
	Victory string      `json:"victory"`        // VictoryHonor ou VictorySword
	Tie     bool        `json:"tie"`            // égalité de points en tête, résolue par la règle de départage
	Note    string      `json:"note,omitempty"` // explication d'un départage ou d'une pénalité
}

// defeat décrit une défaite en cours de traitement.
type defeat struct{ Victim, Killer *Player }

func sameTeam(a, b *Player) bool {
	return a != nil && b != nil && a.Role != nil && b.Role != nil && a.Role.Camp == b.Role.Camp
}

// campRank donne la priorité de départage : Ninja > Shogun/Samouraï > Ronin.
func campRank(camp string) int {
	switch camp {
	case "ninja":
		return 0
	case "shogun_samourai":
		return 1
	}
	return 2
}

// multiplierFor retourne le multiplicateur du rôle pour ce nombre de joueurs.
func multiplierFor(role Role, playerCount int, maxNinjaStars int) (int, string) {
	loadOnce.Do(loadData)
	switch v := roleMult[role.ID][strconv.Itoa(playerCount)].(type) {
	case float64:
		return int(v), ""
	case string: // "etoiles" : ×2 pour le Ninja qui a le plus d'étoiles, sinon ×1
		if role.Stars == maxNinjaStars {
			return 2, "le plus d'étoiles"
		}
		return 1, "pas le plus d'étoiles"
	}
	return 1, ""
}

// campLabel est le nom d'un camp dans les messages.
func campLabel(camp string) string {
	switch camp {
	case "ninja":
		return "les Ninjas"
	case "ronin":
		return "le Ronin"
	}
	return "le Shogun et les Samouraïs"
}

// computeResults calcule le décompte final (verrou déjà pris).
func (g *Game) computeResults(reason string) {
	players := g.sortedPlayers()
	maxStars := 0
	for _, p := range players {
		if p.Role != nil && p.Role.Camp == "ninja" && p.Role.Stars > maxStars {
			maxStars = p.Role.Stars
		}
	}

	res := &Results{Reason: reason, Victory: VictoryHonor}
	camps := map[string]int{}
	for _, p := range players {
		if p.Role == nil || p.Character == nil {
			continue
		}
		mult, note := multiplierFor(*p.Role, len(players), maxStars)
		daimyo := 0
		if p.Role.Camp != "ronin" { // le Ronin ne gagne rien avec les Daimyō
			daimyo = countCards(p.Hand, "daimyo")
		}
		row := ScoreRow{Name: p.Name, Character: p.Character.Name, Role: *p.Role, Honor: p.Honor, Mult: mult, MultNote: note, Daimyo: daimyo, Score: p.Honor*mult + daimyo}
		res.Rows = append(res.Rows, row)
		camps[p.Role.Camp] += row.Score
	}

	// Lame perdue : la partie se termine parce qu'un joueur a été vaincu par un coéquipier.
	penalty := map[string]int{}
	if d := g.lastDefeat; d != nil && sameTeam(d.Victim, d.Killer) {
		camp := d.Victim.Role.Camp
		camps[camp] -= lostBladePenalty
		penalty[camp] = lostBladePenalty
		res.Note = fmt.Sprintf("Lame perdue : %s a été vaincu·e par un coéquipier, son camp perd %d points d'honneur.", d.Victim.Name, lostBladePenalty)
	}

	for camp, total := range camps {
		res.Camps = append(res.Camps, CampScore{Camp: camp, Total: total, Penalty: penalty[camp]})
	}
	sort.Slice(res.Camps, func(i, j int) bool {
		if res.Camps[i].Total != res.Camps[j].Total {
			return res.Camps[i].Total > res.Camps[j].Total
		}
		return campRank(res.Camps[i].Camp) < campRank(res.Camps[j].Camp)
	})
	if len(res.Camps) > 0 {
		res.Winner = res.Camps[0].Camp
	}
	if len(res.Camps) > 1 && res.Camps[0].Total == res.Camps[1].Total {
		res.Tie = true
		tie := fmt.Sprintf("Égalité à %d points : %s l'emporte (départage Ninjas, puis Shogun et Samouraïs, puis Ronin).", res.Camps[0].Total, campLabel(res.Winner))
		if res.Note != "" {
			res.Note += " "
		}
		res.Note += tie
	}

	// Victoire au fil de l'épée : le camp du dernier joueur debout gagne, quels que soient les points.
	if g.sword != "" {
		if p := g.Players[g.sword]; p != nil && p.Role != nil {
			res.Victory = VictorySword
			res.Winner = p.Role.Camp
			res.Tie = false
			res.Note = fmt.Sprintf("Victoire au fil de l'épée : %s est le seul joueur à avoir encore des points de vie.", p.Name)
		}
	}
	g.Results = res
}

// standing retourne les joueurs qui ont encore au moins 1 PV.
func (g *Game) standing() []*Player {
	var out []*Player
	for _, p := range g.sortedPlayers() {
		if p.Life > 0 {
			out = append(out, p)
		}
	}
	return out
}

// checkEnd applique les conditions de fin de partie après une défaite (verrou déjà pris) :
//  1. victoire au fil de l'épée (4 joueurs ou plus) si un seul joueur a encore des PV,
//     sauf si la défaite vient d'un coéquipier : la partie se termine alors, mais aux points ;
//  2. un joueur n'a plus de point d'honneur.
func (g *Game) checkEnd() {
	if g.State != GameStateStarted {
		return
	}
	standing := g.standing()
	teamKill := g.lastDefeat != nil && sameTeam(g.lastDefeat.Victim, g.lastDefeat.Killer)
	sword := len(g.Players) > 3 && len(standing) == 1

	if sword && !teamKill {
		g.sword = standing[0].Name
		g.finish(fmt.Sprintf("%s est le dernier joueur debout.", standing[0].Name))
		return
	}
	g.checkHonor()
	if g.State == GameStateStarted && sword {
		g.finish(fmt.Sprintf("%s est le dernier joueur debout, mais la défaite vient d'un coéquipier.", standing[0].Name))
	}
}

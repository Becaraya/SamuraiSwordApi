package game

import (
	"fmt"
	"sort"
	"strconv"
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

// CampScore est le total d'un camp (shogun_samourai, ninja ou ronin).
type CampScore struct {
	Camp  string `json:"camp"`
	Total int    `json:"total"`
}

// Results explique le calcul des scores : honneur de base, multiplicateur du rôle, bonus Daimyō.
type Results struct {
	Reason string      `json:"reason"`
	Rows   []ScoreRow  `json:"rows"`
	Camps  []CampScore `json:"camps"`
	Tie    bool        `json:"tie"` // le départage n'est pas défini dans la spécification
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

// computeResults calcule le décompte final (verrou déjà pris).
func (g *Game) computeResults(reason string) {
	players := g.sortedPlayers()
	maxStars := 0
	for _, p := range players {
		if p.Role != nil && p.Role.Camp == "ninja" && p.Role.Stars > maxStars {
			maxStars = p.Role.Stars
		}
	}

	res := &Results{Reason: reason}
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
	for camp, total := range camps {
		res.Camps = append(res.Camps, CampScore{Camp: camp, Total: total})
	}
	sort.Slice(res.Camps, func(i, j int) bool {
		if res.Camps[i].Total != res.Camps[j].Total {
			return res.Camps[i].Total > res.Camps[j].Total
		}
		return res.Camps[i].Camp < res.Camps[j].Camp
	})
	res.Tie = len(res.Camps) > 1 && res.Camps[0].Total == res.Camps[1].Total
	g.Results = res
}

// checkLastStanding termine la partie quand un seul joueur a encore des PV.
func (g *Game) checkLastStanding() {
	if g.State != GameStateStarted {
		return
	}
	var standing []*Player
	for _, p := range g.sortedPlayers() {
		if p.Life > 0 {
			standing = append(standing, p)
		}
	}
	if len(standing) == 1 {
		g.finish(fmt.Sprintf("%s est le dernier joueur debout.", standing[0].Name))
	}
}

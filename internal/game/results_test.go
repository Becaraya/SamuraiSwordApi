package game

import "testing"

func TestResultsScoringMultipliersAndDaimyo(t *testing.T) {
	g := table7()
	loadOnce.Do(loadData)
	roleOf := map[string]string{"Aiko": "ninja_1", "Kenji": "shogun", "Sayuri": "samourai", "Ryo": "ninja_2", "Mizuki": "samourai", "Takeshi": "ninja_3", "Haruka": "ronin"}
	honor := map[string]int{"Aiko": 4, "Kenji": 3, "Sayuri": 4, "Ryo": 4, "Mizuki": 3, "Takeshi": 0, "Haruka": 3}
	for name, id := range roleOf {
		for _, r := range roles {
			if r.ID == id {
				r := r
				g.Players[name].Role = &r
			}
		}
	}
	for n, h := range honor {
		g.Players[n].Honor = h
	}
	g.Players["Aiko"].Hand = append(g.Players["Aiko"].Hand, daimyo, daimyo)
	g.Players["Haruka"].Hand = append(g.Players["Haruka"].Hand, daimyo)

	g.finish("Takeshi a atteint 0 point d'honneur.")

	if g.State != GameStateEnded || g.Results == nil {
		t.Fatal("game should be over with results")
	}
	rows := map[string]ScoreRow{}
	for _, r := range g.Results.Rows {
		rows[r.Name] = r
	}
	// À 7 joueurs : Ronin ×3, les autres ×1. Daimyō : +1 chacun, non multiplié, rien pour le Ronin.
	if rows["Haruka"].Mult != 3 || rows["Haruka"].Daimyo != 0 || rows["Haruka"].Score != 9 {
		t.Errorf("Ronin: %+v", rows["Haruka"])
	}
	if rows["Aiko"].Daimyo != 2 || rows["Aiko"].Score != 6 {
		t.Errorf("Aiko: %+v", rows["Aiko"])
	}
	want := map[string]int{"shogun_samourai": 10, "ninja": 10, "ronin": 9}
	for _, c := range g.Results.Camps {
		if c.Total != want[c.Camp] {
			t.Errorf("camp %s = %d, want %d", c.Camp, c.Total, want[c.Camp])
		}
	}
	if !g.Results.Tie {
		t.Error("a tie must be flagged, not resolved silently")
	}
}

func TestFourPlayerNinjaStarsMultiplier(t *testing.T) {
	two, three := Role{ID: "ninja_2", Camp: "ninja", Stars: 2}, Role{ID: "ninja_3", Camp: "ninja", Stars: 3}
	if m, _ := multiplierFor(two, 4, 3); m != 1 {
		t.Errorf("ninja_2 without the most stars should be x1, got %d", m)
	}
	if m, _ := multiplierFor(two, 4, 2); m != 2 {
		t.Errorf("ninja_2 with the most stars should be x2, got %d", m)
	}
	if m, _ := multiplierFor(three, 4, 3); m != 2 {
		t.Errorf("ninja_3 should be x2 at 4 players, got %d", m)
	}
}

func TestLastStandingEndsTheGame(t *testing.T) {
	g := table7()
	for _, n := range []string{"Kenji", "Sayuri", "Ryo", "Takeshi"} {
		g.Players[n].Life = 0
	}
	g.Players["Haruka"].Life = 0
	g.Players["Aiko"].Hand = []Card{katana}
	g.Players["Takeshi"].Life = 1
	g.Pending = &Attack{Attacker: "Aiko", Target: "Takeshi", Weapon: katana, Damage: Calc{Total: 3}}
	g.resolveAttack()
	if g.State != GameStateEnded || g.Results == nil {
		t.Fatalf("Aiko is the last one standing: state=%s", g.State)
	}
}

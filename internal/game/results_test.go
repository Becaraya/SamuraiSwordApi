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
	if !g.Results.Tie || g.Results.Winner != "ninja" || g.Results.Victory != VictoryHonor {
		t.Errorf("tie between ninjas and another team: ninjas win, got %+v", g.Results)
	}
}

func assignRoles(g *Game, roleOf map[string]string) {
	loadOnce.Do(loadData)
	for name, id := range roleOf {
		for _, r := range roles {
			if r.ID == id {
				r := r
				g.Players[name].Role = &r
			}
		}
	}
}

func campTotals(r *Results) map[string]CampScore {
	out := map[string]CampScore{}
	for _, c := range r.Camps {
		out[c.Camp] = c
	}
	return out
}

func attackFor(g *Game, att, tgt string) {
	g.Pending = &Attack{Attacker: att, Target: tgt, Weapon: katana, Damage: Calc{Total: 3}}
	g.resolveAttack()
}

func TestTieBetweenRoninAndShogunTeamGoesToShogunTeam(t *testing.T) {
	g := table7()
	assignRoles(g, map[string]string{"Aiko": "ninja_1", "Kenji": "shogun", "Sayuri": "samourai", "Ryo": "ninja_2", "Mizuki": "samourai", "Takeshi": "ninja_3", "Haruka": "ronin"})
	for n, h := range map[string]int{"Aiko": 1, "Kenji": 3, "Sayuri": 2, "Ryo": 1, "Mizuki": 1, "Takeshi": 1, "Haruka": 2} {
		g.Players[n].Honor = h
	}
	g.finish("test")
	c := campTotals(g.Results)
	if c["shogun_samourai"].Total != 6 || c["ronin"].Total != 6 {
		t.Fatalf("totals: %+v", g.Results.Camps)
	}
	if !g.Results.Tie || g.Results.Winner != "shogun_samourai" {
		t.Errorf("shogun/samurai must beat the ronin on a tie: %+v", g.Results)
	}
}

func TestLostBladePenalisesTheVictimsTeam(t *testing.T) {
	g := table7()
	assignRoles(g, map[string]string{"Aiko": "ninja_1", "Kenji": "shogun", "Sayuri": "samourai", "Ryo": "ninja_2", "Mizuki": "samourai", "Takeshi": "ninja_3", "Haruka": "ronin"})
	g.Players["Takeshi"].Life, g.Players["Takeshi"].Honor = 1, 1
	attackFor(g, "Aiko", "Takeshi") // un Ninja vainc un Ninja qui n'a plus qu'un point d'honneur
	if g.State != GameStateEnded || g.Results == nil {
		t.Fatalf("game should end: %s", g.State)
	}
	if g.Results.Victory != VictoryHonor {
		t.Errorf("no swordmaster victory expected: %+v", g.Results)
	}
	nin := campTotals(g.Results)["ninja"]
	// Aiko 5 (4+1), Ryo 4, Takeshi 0 : 9 points, moins 3.
	if nin.Penalty != 3 || nin.Total != 6 {
		t.Errorf("ninja camp: %+v", nin)
	}
}

func TestNoPenaltyWhenAnEnemyDefeatsYou(t *testing.T) {
	g := table7()
	assignRoles(g, map[string]string{"Aiko": "ninja_1", "Kenji": "shogun", "Sayuri": "samourai", "Ryo": "ninja_2", "Mizuki": "samourai", "Takeshi": "ninja_3", "Haruka": "ronin"})
	g.Players["Sayuri"].Life, g.Players["Sayuri"].Honor = 1, 1
	attackFor(g, "Aiko", "Sayuri")
	if g.State != GameStateEnded {
		t.Fatalf("game should end: %s", g.State)
	}
	for _, c := range g.Results.Camps {
		if c.Penalty != 0 {
			t.Errorf("unexpected penalty: %+v", c)
		}
	}
}

func TestSwordmasterVictoryOverridesPoints(t *testing.T) {
	g := table7()
	assignRoles(g, map[string]string{"Aiko": "ninja_1", "Kenji": "shogun", "Sayuri": "samourai", "Ryo": "ninja_2", "Mizuki": "samourai", "Takeshi": "ninja_3", "Haruka": "ronin"})
	for _, n := range []string{"Kenji", "Sayuri", "Ryo", "Mizuki", "Haruka"} {
		g.Players[n].Life = 0
	}
	g.Players["Takeshi"].Life = 1
	// Le camp du Shogun est largement en tête aux points.
	g.Players["Kenji"].Honor, g.Players["Sayuri"].Honor, g.Players["Mizuki"].Honor = 9, 9, 9
	// Takeshi (ninja) est vaincu par Aiko (ninja) : coup d'équipe.
	attackFor(g, "Aiko", "Takeshi")
	if g.Results == nil || g.Results.Victory != VictoryHonor {
		t.Fatalf("a team kill must not give the swordmaster victory: %+v", g.Results)
	}

	g2 := table7()
	assignRoles(g2, map[string]string{"Aiko": "ninja_1", "Kenji": "shogun", "Sayuri": "samourai", "Ryo": "ninja_2", "Mizuki": "samourai", "Takeshi": "ninja_3", "Haruka": "ronin"})
	for _, n := range []string{"Kenji", "Ryo", "Mizuki", "Haruka", "Takeshi"} {
		g2.Players[n].Life = 0
	}
	g2.Players["Sayuri"].Life = 1
	g2.Players["Kenji"].Honor, g2.Players["Mizuki"].Honor = 9, 9
	attackFor(g2, "Aiko", "Sayuri")
	if g2.Results == nil || g2.Results.Victory != VictorySword || g2.Results.Winner != "ninja" {
		t.Fatalf("Aiko is the only one standing, her team wins: %+v", g2.Results)
	}
}

func TestNoSwordmasterVictoryAtThreePlayers(t *testing.T) {
	g := table7()
	for _, n := range []string{"Ryo", "Mizuki", "Takeshi", "Haruka"} {
		delete(g.Players, n)
	}
	assignRoles(g, map[string]string{"Aiko": "ninja_1", "Kenji": "ninja_2", "Sayuri": "shogun"})
	g.Players["Kenji"].Life = 0
	g.Players["Sayuri"].Life = 1
	attackFor(g, "Aiko", "Sayuri")
	if g.State != GameStateStarted {
		t.Fatalf("with 3 players the game goes on, got %s", g.State)
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
	assignRoles(g, map[string]string{"Aiko": "ninja_1", "Takeshi": "shogun"})
	for _, n := range []string{"Kenji", "Sayuri", "Ryo", "Takeshi"} {
		g.Players[n].Life = 0
	}
	g.Players["Haruka"].Life = 0
	g.Players["Aiko"].Hand = []Card{katana}
	g.Players["Takeshi"].Life = 1
	g.Pending = &Attack{Attacker: "Aiko", Target: "Takeshi", Weapon: katana, Damage: Calc{Total: 3}}
	g.resolveAttack()
	if g.State != GameStateEnded || g.Results == nil || g.Results.Victory != VictorySword {
		t.Fatalf("Aiko is the last one standing: state=%s %+v", g.State, g.Results)
	}
}

package game

import "testing"

var (
	cri       = Card{ID: "cri_de_guerre", Name: "Cri de guerre", Category: "action"}
	jujitsu   = Card{ID: "ju_jitsu", Name: "Ju Jitsu", Category: "action"}
	diversion = Card{ID: "diversion", Name: "Diversion", Category: "action"}
	daimyo    = Card{ID: "daimyo", Name: "Daimyo", Category: "action"}
	bushido   = Card{ID: "code_du_bushido", Name: "Code du Bushido", Category: "permanente"}
)

func TestCriDeGuerreCollectiveResolution(t *testing.T) {
	g := table7()
	aiko := g.Players["Aiko"]
	aiko.Hand = []Card{cri, katana}
	g.Players["Kenji"].Hand = []Card{parade, geisha}
	g.Players["Sayuri"].Hand = []Card{geisha, geisha}
	g.Players["Takeshi"].Hand = []Card{geisha}
	g.refresh()

	if err := g.PlayCard("Aiko", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	rows := map[string]CollectiveRow{}
	for _, r := range g.Collective.Rows {
		rows[r.Name] = r
	}
	// Mizuki (0 PV) et Haruka (sans carte) sont à terre, Ryo est Chiyome : tous exemptés.
	for _, n := range []string{"Mizuki", "Haruka", "Ryo"} {
		if rows[n].Status != "exempt" {
			t.Errorf("%s should be exempt, got %+v", n, rows[n])
		}
	}
	// Sayuri (Hanzo) n'a ni parade ni arme : elle perd 1 PV sans décision. Takeshi aussi.
	if rows["Sayuri"].Status != "done" || g.Players["Sayuri"].Life != 2 {
		t.Errorf("Sayuri should lose 1 HP automatically: %+v life=%d", rows["Sayuri"], g.Players["Sayuri"].Life)
	}
	if rows["Kenji"].Status != "pending" || !g.collectivePending("Kenji") {
		t.Fatalf("Kenji holds a parry and must decide: %+v", rows["Kenji"])
	}
	if err := g.EndTurn("Aiko"); err != ErrAttackPending {
		t.Fatalf("turn must wait for the collective effect, got %v", err)
	}
	if err := g.Respond("Kenji", true, 1); err != ErrInvalidParry {
		t.Fatalf("a non-parry card must be refused, got %v", err)
	}
	if err := g.Respond("Kenji", true, 0); err != nil {
		t.Fatal(err)
	}
	if g.Collective != nil || g.Players["Kenji"].Life != 5 {
		t.Fatalf("effect should be resolved without damage for Kenji: %+v", g.Collective)
	}
}

func TestJuJitsuAcceptsOnlyWeapons(t *testing.T) {
	g := table7()
	g.Players["Aiko"].Hand = []Card{jujitsu}
	g.Players["Kenji"].Hand = []Card{parade, bokken}
	g.refresh()
	if err := g.PlayCard("Aiko", 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := g.Respond("Kenji", true, 0); err != ErrInvalidParry {
		t.Fatalf("a parry cannot answer Ju Jitsu, got %v", err)
	}
	if err := g.Respond("Kenji", true, 1); err != nil {
		t.Fatal(err)
	}
}

func TestDiversionGeishaAndDaimyo(t *testing.T) {
	g := table7()
	aiko := g.Players["Aiko"]
	aiko.Hand = []Card{diversion, geisha, daimyo, armure}
	g.Players["Haruka"].Hand = nil
	g.refresh()

	if err := g.PlayCard("Aiko", 0, "Haruka", ""); err != ErrInvalidTarget {
		t.Fatalf("a player without cards cannot be targeted by Diversion, got %v", err)
	}
	if err := g.PlayCard("Aiko", 0, "Kenji", ""); err != nil {
		t.Fatal(err)
	}
	if len(aiko.Hand) != 4 || len(g.Players["Kenji"].Hand) != 2 {
		t.Fatalf("Diversion should move one card: %d / %d", len(aiko.Hand), len(g.Players["Kenji"].Hand))
	}

	// Geisha : une permanente en jeu (Armure de Kenji) ou une carte au hasard.
	for i, c := range aiko.Hand {
		if c.ID == "geisha" {
			if err := g.PlayCard("Aiko", i, "Kenji", "bogus"); err != ErrInvalidTarget {
				t.Fatalf("unknown option must be refused, got %v", err)
			}
			if err := g.PlayCard("Aiko", i, "Kenji", "perm:0"); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if len(g.Players["Kenji"].Permanents) != 0 {
		t.Fatal("Armure should have been discarded")
	}

	before := len(aiko.Hand)
	for i, c := range aiko.Hand {
		if c.ID == "daimyo" {
			_ = g.PlayCard("Aiko", i, "", "")
			break
		}
	}
	if len(aiko.Hand) != before+1 { // -1 carte jouée, +2 piochées
		t.Fatalf("Daimyo should draw 2 cards: %d -> %d", before, len(aiko.Hand))
	}
}

func TestBushidoWeaponChoiceAndAutomaticPass(t *testing.T) {
	g := table7()
	g.Players["Aiko"].Permanents = append(g.Players["Aiko"].Permanents, bushido)
	g.Active = "Haruka"
	g.Deck = []Card{katana, geisha, geisha, geisha, geisha, geisha}

	// Tour d'Aiko : le Code révèle une arme -> décision.
	g.Active = "Haruka"
	g.nextTurn()
	if g.Active != "Aiko" || g.Bushido == nil || g.Phase != PhaseRecover {
		t.Fatalf("Aiko must decide: active=%s phase=%s bushido=%v", g.Active, g.Phase, g.Bushido)
	}
	if g.Bushido.Next != "Kenji" {
		t.Fatalf("the Code goes to the left neighbour, got %s", g.Bushido.Next)
	}
	if err := g.EndTurn("Aiko"); err == nil {
		t.Fatal("cannot end the turn while the choice is pending")
	}
	honor := g.Players["Aiko"].Honor
	if err := g.Respond("Aiko", true, 2); err != ErrInvalidParry {
		t.Fatalf("only a weapon can be discarded, got %v", err)
	}
	if err := g.Respond("Aiko", true, 0); err != nil {
		t.Fatal(err)
	}
	if g.Phase != PhasePlay || countCards(g.Players["Kenji"].Permanents, "code_du_bushido") != 1 || g.Players["Aiko"].Honor != honor {
		t.Fatal("Code should move to Kenji and the turn should continue")
	}

	// Carte révélée non arme : le Code passe tout seul.
	g.Players["Kenji"].Life = 5
	g.Active = "Aiko"
	g.Deck = []Card{geisha, geisha, geisha, geisha, geisha, geisha}
	g.nextTurn()
	if g.Active != "Kenji" || g.Bushido != nil || countCards(g.Players["Sayuri"].Permanents, "code_du_bushido") != 1 {
		t.Fatalf("Code should pass automatically to Sayuri: active=%s", g.Active)
	}
}

func TestBushidoPayHonor(t *testing.T) {
	g := table7()
	g.Players["Aiko"].Permanents = append(g.Players["Aiko"].Permanents, bushido)
	g.Deck = []Card{katana, geisha, geisha, geisha, geisha, geisha}
	g.Active = "Haruka"
	g.nextTurn()
	honor := g.Players["Aiko"].Honor
	if err := g.Respond("Aiko", false, 0); err != nil {
		t.Fatal(err)
	}
	if g.Players["Aiko"].Honor != honor-1 || g.codeHolder() != nil || g.Phase != PhasePlay {
		t.Fatalf("honor=%d holder=%v phase=%s", g.Players["Aiko"].Honor, g.codeHolder(), g.Phase)
	}
}

func TestOnlyOneBushidoInPlay(t *testing.T) {
	g := table7()
	g.Players["Kenji"].Permanents = append(g.Players["Kenji"].Permanents, bushido)
	g.Players["Aiko"].Hand = []Card{bushido}
	if p := g.playability(g.Players["Aiko"], bushido); p.OK {
		t.Fatal("a second Code du Bushido must be refused")
	}
}

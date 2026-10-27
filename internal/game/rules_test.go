package game

import "testing"

var (
	katana  = Card{ID: "katana", Name: "Katana", Category: "arme", Precision: 2, Damage: 3}
	bokken  = Card{ID: "bokken", Name: "Bokken", Category: "arme", Precision: 1, Damage: 1}
	parade  = Card{ID: "parade", Name: "Parade", Category: "action"}
	geisha  = Card{ID: "geisha", Name: "Geisha", Category: "action"}
	armure  = Card{ID: "armure", Name: "Armure", Category: "permanente"}
	quick   = Card{ID: "attaque_rapide", Name: "Attaque rapide", Category: "permanente"}
	conc    = Card{ID: "concentration", Name: "Concentration", Category: "permanente"}
	hands3  = []Card{geisha, geisha, geisha}
	roleNin = &Role{ID: "ninja", Name: "Ninja"}
)

// table7 reproduit la situation du prototype UX : Aiko (Goemon) attaque depuis la place 1.
func table7() *Game {
	g := NewGame("Aiko")
	add := func(pos int, name, char string, life, maxLife int, hand []Card, perms ...Card) {
		g.Players[name] = &Player{
			Name: name, Position: pos, Life: life, MaxLife: maxLife, Honor: 4,
			Character: &Character{ID: char, Name: char}, Role: roleNin,
			Hand: hand, Permanents: perms,
		}
	}
	add(1, "Aiko", "goemon", 5, 5, []Card{katana, bokken, parade, geisha}, conc)
	add(2, "Kenji", "benkei", 5, 5, hands3, armure)
	add(3, "Sayuri", "hanzo", 3, 4, []Card{geisha, geisha, parade})
	add(4, "Ryo", "chiyome", 4, 4, hands3, armure)
	add(5, "Mizuki", "hideyoshi", 0, 4, hands3)
	add(6, "Takeshi", "musashi", 5, 5, hands3, quick)
	add(7, "Haruka", "ushiwaka", 2, 4, nil)
	g.State = GameStateStarted
	g.Active, g.Phase, g.Turn = "Aiko", PhasePlay, 5
	g.Deck = []Card{geisha, geisha, geisha, geisha, geisha, geisha}
	g.refresh()
	return g
}

func TestWeaponTargetsMatchPrototype(t *testing.T) {
	g := table7()
	want := map[string]struct {
		ok   bool
		diff int
	}{"Kenji": {false, 3}, "Sayuri": {true, 2}, "Ryo": {false, 3}, "Mizuki": {false, 0}, "Takeshi": {true, 1}, "Haruka": {false, 0}}
	for _, tg := range g.weaponTargets(g.Players["Aiko"], katana) {
		w := want[tg.Name]
		if tg.OK != w.ok || (tg.Difficulty != nil && tg.Difficulty.Total != w.diff) {
			t.Errorf("%s: ok=%v diff=%+v, want ok=%v diff=%d", tg.Name, tg.OK, tg.Difficulty, w.ok, w.diff)
		}
	}
	if limit := g.weaponLimit(g.Players["Aiko"]); limit.Total != 3 {
		t.Errorf("Goemon + Concentration should allow 3 weapons, got %d", limit.Total)
	}
	if g.Players["Mizuki"].Down != "hp" || g.Players["Haruka"].Down != "hand" {
		t.Errorf("down causes: %q / %q", g.Players["Mizuki"].Down, g.Players["Haruka"].Down)
	}
}

func TestParryCancelsDamage(t *testing.T) {
	g := table7()
	if err := g.PlayCard("Aiko", 0, "Sayuri", ""); err != nil {
		t.Fatal(err)
	}
	if g.Pending == nil || g.Pending.Damage.Total != 3 {
		t.Fatalf("attack should be pending with 3 damage: %+v", g.Pending)
	}
	if err := g.EndTurn("Aiko"); err != ErrAttackPending {
		t.Fatalf("turn cannot end during an attack, got %v", err)
	}
	if err := g.Respond("Kenji", true, 0); err != ErrNotTarget {
		t.Fatalf("only the target may respond, got %v", err)
	}
	if err := g.Respond("Sayuri", true, 0); err != ErrInvalidParry {
		t.Fatalf("a non-parry card must be refused, got %v", err)
	}
	// Hanzo peut aussi parer avec une arme, mais ici sa main ne contient qu'une Parade à l'indice 2.
	if err := g.Respond("Sayuri", true, 2); err != nil {
		t.Fatal(err)
	}
	if g.Pending != nil || g.Players["Sayuri"].Life != 3 || g.Played != 1 {
		t.Fatalf("attack should be cancelled: pending=%v life=%d played=%d", g.Pending, g.Players["Sayuri"].Life, g.Played)
	}
}

func TestUnparriedKillTransfersHonorAndUpdatesTargets(t *testing.T) {
	g := table7()
	g.Players["Sayuri"].Hand = []Card{geisha, geisha}
	if err := g.PlayCard("Aiko", 0, "Sayuri", ""); err != nil {
		t.Fatal(err)
	}
	// Aucune parade : résolution automatique.
	sayuri := g.Players["Sayuri"]
	if g.Pending != nil || sayuri.Life != 0 || sayuri.Down != "hp" {
		t.Fatalf("Sayuri should be down: pending=%v life=%d down=%s", g.Pending, sayuri.Life, sayuri.Down)
	}
	if sayuri.Honor != 3 || g.Players["Aiko"].Honor != 5 {
		t.Fatalf("honor transfer failed: %d / %d", sayuri.Honor, g.Players["Aiko"].Honor)
	}
	// Sayuri ne compte plus : Ryo devient plus proche mais son armure garde la difficulté à 3 → toujours hors de portée du Katana.
	for _, tg := range g.weaponTargets(g.Players["Aiko"], bokken) {
		if tg.Name == "Sayuri" && tg.OK {
			t.Fatal("a player who is down cannot be targeted")
		}
	}
}

func TestWeaponLimitAndPlayability(t *testing.T) {
	g := table7()
	g.Played = 3
	if p := g.playability(g.Players["Aiko"], katana); p.OK {
		t.Fatal("limit reached: weapon should be unplayable")
	}
	g.Played = 0
	if p := g.playability(g.Players["Aiko"], parade); p.OK {
		t.Fatal("a parry cannot be played on your own turn")
	}
	if err := g.PlayCard("Aiko", 0, "Haruka", ""); err != ErrInvalidTarget {
		t.Fatalf("down target must be refused, got %v", err)
	}
	if err := g.PlayCard("Kenji", 0, "Aiko", ""); err != ErrNotYourTurn {
		t.Fatalf("expected ErrNotYourTurn, got %v", err)
	}
}

func TestBotAttacksHumanThenTurnPasses(t *testing.T) {
	g := table7()
	g.Active = "Takeshi"
	g.Players["Takeshi"].Hand = []Card{bokken, geisha, geisha}
	g.Players["Takeshi"].Permanents = nil
	g.Players["Takeshi"].Name = "Bot-1"
	g.Players["Bot-1"] = g.Players["Takeshi"]
	delete(g.Players, "Takeshi")
	g.Active = "Bot-1"

	if !g.NeedsBotStep() {
		t.Fatal("bot should need to act")
	}
	if _, ok := g.PlayBotStep(); !ok || g.Pending == nil && g.Players["Aiko"].Life == 5 {
		t.Fatal("bot should have attacked")
	}
}

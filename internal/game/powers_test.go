package game

import "testing"

func TestIeyasuMayTakeTheTopOfTheDiscard(t *testing.T) {
	g := table7()
	aiko := g.Players["Aiko"]
	aiko.Character = &Character{ID: "ieyasu", Name: "Ieyasu"}
	g.Discard = []Card{geisha, katana}
	g.Deck = []Card{parade, parade, parade, parade}
	aiko.Hand = nil

	g.drawPhase(aiko)
	if g.DrawChoice == nil || g.DrawChoice.Top.ID != "katana" || g.Phase != PhaseDraw {
		t.Fatalf("Ieyasu must be offered the discard: %+v phase=%s", g.DrawChoice, g.Phase)
	}
	if p := g.playability(aiko, katana); p.OK {
		t.Fatal("nothing can be played during the draw choice")
	}
	if err := g.Respond("Kenji", true, 0); err != ErrNotTarget {
		t.Fatalf("only Ieyasu decides, got %v", err)
	}
	if err := g.Respond("Aiko", true, 0); err != nil {
		t.Fatal(err)
	}
	if len(aiko.Hand) != 2 || aiko.Hand[0].ID != "katana" || len(g.Discard) != 1 || g.Phase != PhasePlay {
		t.Fatalf("hand=%v discard=%d phase=%s", aiko.Hand, len(g.Discard), g.Phase)
	}
}

func TestIeyasuNormalDrawAndBotsSkipTheChoice(t *testing.T) {
	g := table7()
	aiko := g.Players["Aiko"]
	aiko.Character = &Character{ID: "ieyasu", Name: "Ieyasu"}
	g.Discard = []Card{katana}
	aiko.Hand = nil
	g.drawPhase(aiko)
	if err := g.Respond("Aiko", false, 0); err != nil || len(aiko.Hand) != 2 || len(g.Discard) != 1 {
		t.Fatalf("normal draw: err=%v hand=%d discard=%d", err, len(aiko.Hand), len(g.Discard))
	}

	bot := g.Players["Takeshi"]
	bot.Name = "Bot-1"
	g.Players["Bot-1"] = bot
	delete(g.Players, "Takeshi")
	bot.Character = &Character{ID: "ieyasu", Name: "Ieyasu"}
	g.drawPhase(bot)
	if g.DrawChoice != nil {
		t.Fatal("bots never wait for a draw choice")
	}
}

func TestNobunagaSacrificesLifeButNeverTheLast(t *testing.T) {
	g := table7()
	aiko := g.Players["Aiko"]
	aiko.Character = &Character{ID: "nobunaga", Name: "Nobunaga"}
	aiko.Life = 2
	hand := len(aiko.Hand)

	if info := g.powerOf(aiko); info == nil || !info.OK {
		t.Fatalf("power should be available: %+v", info)
	}
	if err := g.UsePower("Kenji", "nobunaga"); err != ErrNotPlayable {
		t.Fatalf("only Nobunaga has this power, got %v", err)
	}
	if err := g.UsePower("Aiko", "nobunaga"); err != nil {
		t.Fatal(err)
	}
	if aiko.Life != 1 || len(aiko.Hand) != hand+1 {
		t.Fatalf("life=%d hand=%d", aiko.Life, len(aiko.Hand))
	}
	if err := g.UsePower("Aiko", "nobunaga"); err != ErrNotPlayable {
		t.Fatalf("the last HP cannot be sacrificed, got %v", err)
	}
	if info := g.powerOf(aiko); info.OK || info.Reason == "" {
		t.Fatalf("a reason must be given: %+v", info)
	}
}

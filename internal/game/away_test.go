package game

import "testing"

func TestAwayPlayerIsPlayedByTheServer(t *testing.T) {
	g := table7()
	g.Active = "Kenji"
	if g.NeedsBotStep() {
		t.Fatal("a connected human must not be automated")
	}
	if !g.SetAway("Kenji", true) || g.SetAway("Kenji", true) {
		t.Fatal("SetAway must report the change once")
	}
	if !g.NeedsBotStep() {
		t.Fatal("an absent active player must be automated")
	}
	if name, ok := g.PlayBotStep(); !ok || name != "Kenji" {
		t.Fatalf("server should end Kenji's turn: %q %v", name, ok)
	}
	if g.Active == "Kenji" || g.Played != 0 {
		t.Errorf("turn should have moved on without an attack: active=%s played=%d", g.Active, g.Played)
	}
}

func TestAwayPlayerAnswersAnAttackAndAChoice(t *testing.T) {
	g := table7()
	g.SetAway("Sayuri", true)
	if err := g.PlayCard("Aiko", 0, "Sayuri", ""); err != nil {
		t.Fatal(err)
	}
	if !g.NeedsBotStep() {
		t.Fatal("the absent target must be answered by the server")
	}
	if _, ok := g.PlayBotStep(); !ok || g.Pending != nil {
		t.Fatalf("attack should be resolved: pending=%+v", g.Pending)
	}

	g.DrawChoice = &DrawChoice{Player: "Kenji", Count: 2}
	g.SetAway("Kenji", true)
	if !g.NeedsBotStep() {
		t.Fatal("a pending draw choice of an absent player must be automated")
	}
	if _, ok := g.PlayBotStep(); !ok || g.DrawChoice != nil {
		t.Fatalf("draw choice should be answered: %+v", g.DrawChoice)
	}
}

func TestAwayStateIsClearedAndBotsAreNeverAway(t *testing.T) {
	g := table7()
	g.SetAway("Ryo", true)
	if !g.SetAway("Ryo", false) || g.Players["Ryo"].Away {
		t.Fatal("reconnection must clear the flag")
	}
	g.Players["Bot-1"] = &Player{Name: "Bot-1"}
	if g.SetAway("Bot-1", true) {
		t.Error("bots are always automated, never flagged away")
	}
	if g.AllHumansAway() {
		t.Error("humans are still present")
	}
	for n := range g.Players {
		g.SetAway(n, true)
	}
	if !g.AllHumansAway() {
		t.Error("everybody is away")
	}
}

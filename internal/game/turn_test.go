package game

import "testing"

func startedGame(t *testing.T, n int) *Game {
	t.Helper()
	g := NewGame("p1")
	for i := 1; i <= n; i++ {
		g.AddPlayer(NewPlayer("p"+string(rune('0'+i)), 0))
	}
	if !g.StartGame() {
		t.Fatal("game should start")
	}
	return g
}

func TestStartGameDealsRolesHonorAndTurn(t *testing.T) {
	for _, n := range []int{3, 5, 7} {
		g := startedGame(t, n)
		shogun := g.Players[g.Active]
		if shogun.Role == nil || shogun.Role.ID != RoleShogun {
			t.Fatalf("%d players: the Shogun must start", n)
		}
		if g.Phase != PhasePlay || g.Turn != 1 {
			t.Fatalf("%d players: expected play phase on turn 1, got %s/%d", n, g.Phase, g.Turn)
		}
		wantShogun, wantOthers := startingHonor(n)
		for _, p := range g.Players {
			want := wantOthers
			if p == shogun {
				want = wantShogun
			}
			if p.Honor != want {
				t.Fatalf("%d players: %s honor = %d, want %d", n, p.Name, p.Honor, want)
			}
			if p.HandCount != len(p.Hand) || p.Character == nil {
				t.Fatalf("%d players: %s is not fully dealt", n, p.Name)
			}
		}
		// 4 cartes de départ pour le Shogun + 2 de pioche (3 à 3 joueurs)
		draw := 2
		if n == 3 {
			draw = 3
		}
		if shogun.Character.ID == "hideyoshi" {
			draw++
		}
		if len(shogun.Hand) != startingHandSize(0)+draw {
			t.Fatalf("%d players: Shogun hand = %d, want %d", n, len(shogun.Hand), startingHandSize(0)+draw)
		}
	}
}

func TestTurnFlowWithMandatoryDiscard(t *testing.T) {
	g := startedGame(t, 4)
	first := g.Active

	if err := g.EndTurn("nobody"); err != ErrNotYourTurn {
		t.Fatalf("expected ErrNotYourTurn, got %v", err)
	}

	// Force une main de 9 cartes : la défausse devient obligatoire.
	p := g.Players[first]
	for len(p.Hand) < 9 {
		p.Hand = append(p.Hand, g.Deck[0])
		g.Deck = g.Deck[1:]
	}
	if err := g.EndTurn(first); err != nil || g.Phase != PhaseDiscard {
		t.Fatalf("expected discard phase, got %v / %s", err, g.Phase)
	}
	if err := g.DiscardCards(first, []int{0}); err != ErrInvalidDiscard {
		t.Fatalf("one card is not enough, got %v", err)
	}
	if err := g.DiscardCards(first, []int{0, 1}); err != nil {
		t.Fatal(err)
	}
	if g.DrawChoice != nil { // le joueur suivant peut être Ieyasu
		if err := g.Respond(g.DrawChoice.Player, false, 0); err != nil {
			t.Fatal(err)
		}
	}
	if g.Active == first || g.Turn != 2 || g.Phase != PhasePlay {
		t.Fatalf("turn should have passed on: active=%s turn=%d phase=%s", g.Active, g.Turn, g.Phase)
	}
	if len(p.Hand) != MaxHandSize || g.DiscardCount != 2 {
		t.Fatalf("hand=%d discard=%d", len(p.Hand), g.DiscardCount)
	}
}

func TestReshuffleCostsHonorAndCanEndGame(t *testing.T) {
	g := startedGame(t, 3)
	g.Deck = nil
	g.Discard = []Card{{ID: "parade"}, {ID: "parade"}, {ID: "parade"}}
	for _, p := range g.Players {
		p.Honor = 1
	}
	g.drawCards(g.Players[g.Active], 1)
	if g.State != GameStateEnded {
		t.Fatal("game should end when honor reaches 0")
	}
	for _, p := range g.Players {
		if p.PublicRole == nil {
			t.Fatal("roles must be revealed at the end")
		}
	}
}

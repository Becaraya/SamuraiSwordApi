package game

import (
	"strings"
	"testing"
	"time"
)

func TestPlayCardExpectRejectsAStaleCard(t *testing.T) {
	g := table7()
	if err := g.PlayCardExpect("Aiko", 0, "bokken", "Sayuri", ""); err != ErrCardNotFound {
		t.Fatalf("index 0 is a katana, not a bokken: %v", err)
	}
	if len(g.Players["Aiko"].Hand) != 4 || g.Pending != nil {
		t.Fatal("a stale request must not change the game")
	}
	if err := g.PlayCardExpect("Aiko", 0, "katana", "Sayuri", ""); err != nil || g.Pending == nil {
		t.Fatalf("the expected card must be played: %v", err)
	}
}

func TestRespondExpectRejectsAStaleParry(t *testing.T) {
	g := table7()
	if err := g.PlayCard("Aiko", 0, "Sayuri", ""); err != nil {
		t.Fatal(err)
	}
	if err := g.RespondExpect("Sayuri", true, 2, "geisha"); err != ErrCardNotFound {
		t.Fatalf("index 2 is a parade: %v", err)
	}
	if g.Pending == nil {
		t.Fatal("the attack must stay pending")
	}
	if err := g.RespondExpect("Sayuri", true, 2, "parade"); err != nil || g.Pending != nil {
		t.Fatalf("parry with the expected card: %v", err)
	}
}

func TestChatLimits(t *testing.T) {
	g := table7()
	if err := g.PostChat("Intrus", "salut"); err != ErrNotInGame {
		t.Errorf("only players may write: %v", err)
	}
	if err := g.PostChat("Aiko", strings.Repeat("a", maxChatLength+1)); err != ErrChatInvalid {
		t.Errorf("too long: %v", err)
	}
	if err := g.PostChat("Aiko", " bonjour "); err != nil || g.Chat[0].Text != "bonjour" {
		t.Fatalf("trimmed message expected: %v %+v", err, g.Chat)
	}
	if err := g.PostChat("Aiko", "encore"); err != ErrChatRate {
		t.Errorf("rate limit: %v", err)
	}
	g.Chat[0].At = time.Now().Add(-time.Second)
	for i := 0; i < maxChatMessages+5; i++ {
		g.Chat = append(g.Chat, ChatMessage{From: "Kenji", Text: "x", At: time.Now().Add(-time.Minute)})
	}
	if err := g.PostChat("Aiko", "suite"); err != nil || len(g.Chat) != maxChatMessages {
		t.Errorf("history must be capped: %v len=%d", err, len(g.Chat))
	}
}

func TestCatalogListsEveryCard(t *testing.T) {
	cards, chars := Catalog()
	total := 0
	for _, c := range cards {
		total += c.Quantity
	}
	if len(cards) == 0 || len(chars) == 0 || total < 90 {
		t.Errorf("catalog looks incomplete: %d cards (%d copies), %d characters", len(cards), total, len(chars))
	}
}

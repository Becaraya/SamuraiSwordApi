package game

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrChatInvalid = errors.New("message vide ou trop long")
	ErrChatRate    = errors.New("trop de messages, patientez un instant")
	ErrNotInGame   = errors.New("seuls les joueurs peuvent écrire")
)

const (
	maxChatMessages = 50
	maxChatLength   = 200
	chatMinGap      = 700 * time.Millisecond
)

// ChatMessage est un message public du chat de la table.
type ChatMessage struct {
	From string    `json:"from"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// PostChat ajoute un message de joueur au chat (limité en taille et en fréquence).
func (g *Game) PostChat(name, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.Players[name]; !ok || isBotName(name) {
		return ErrNotInGame
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxChatLength {
		return ErrChatInvalid
	}
	now := time.Now()
	for i := len(g.Chat) - 1; i >= 0; i-- {
		if g.Chat[i].From == name {
			if now.Sub(g.Chat[i].At) < chatMinGap {
				return ErrChatRate
			}
			break
		}
	}
	g.Chat = append(g.Chat, ChatMessage{From: name, Text: text, At: now})
	if len(g.Chat) > maxChatMessages {
		g.Chat = g.Chat[len(g.Chat)-maxChatMessages:]
	}
	return nil
}

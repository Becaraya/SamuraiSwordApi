package game

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"

	"github.com/becaraya/katana-api/assets"
)

// StartingHandSize est le nombre de cartes distribuées à chaque joueur au lancement.
const StartingHandSize = 4

// Card représente une carte du deck.
type Card struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Precision int    `json:"precision,omitempty"`
	Damage    int    `json:"damage,omitempty"`
	Effect    string `json:"effect,omitempty"`
}

// Character représente le personnage incarné par un joueur.
type Character struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Life    int    `json:"life"`
	Ability string `json:"ability"`
}

type cardDefinition struct {
	Card
	Quantity int
}

var (
	loadOnce    sync.Once
	cardDefs    []cardDefinition
	characters  []Character
	loadFailure error
)

func loadData() {
	var rawCards struct {
		Cards []struct {
			ID        string `json:"id"`
			Name      string `json:"nom"`
			Category  string `json:"categorie"`
			Quantity  int    `json:"quantite"`
			Precision int    `json:"precision"`
			Damage    int    `json:"degats"`
			Effect    string `json:"effet"`
		} `json:"cartes"`
	}
	var rawCharacters struct {
		Characters []struct {
			ID      string `json:"id"`
			Name    string `json:"nom"`
			Life    int    `json:"points_de_vie"`
			Ability string `json:"capacite"`
		} `json:"personnages"`
	}

	for file, target := range map[string]any{
		"data/cards.json":      &rawCards,
		"data/characters.json": &rawCharacters,
	} {
		content, err := assets.Data.ReadFile(file)
		if err == nil {
			err = json.Unmarshal(content, target)
		}
		if err != nil {
			loadFailure = fmt.Errorf("%s: %w", file, err)
			return
		}
	}

	for _, c := range rawCards.Cards {
		cardDefs = append(cardDefs, cardDefinition{
			Card:     Card{ID: c.ID, Name: c.Name, Category: c.Category, Precision: c.Precision, Damage: c.Damage, Effect: c.Effect},
			Quantity: c.Quantity,
		})
	}
	for _, c := range rawCharacters.Characters {
		characters = append(characters, Character{ID: c.ID, Name: c.Name, Life: c.Life, Ability: c.Ability})
	}
}

// buildDeck retourne un deck complet mélangé.
func buildDeck() []Card {
	loadOnce.Do(loadData)
	if loadFailure != nil {
		panic("katana: données de jeu invalides: " + loadFailure.Error())
	}

	deck := make([]Card, 0, 100)
	for _, def := range cardDefs {
		for i := 0; i < def.Quantity; i++ {
			deck = append(deck, def.Card)
		}
	}
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}

// shuffledCharacters retourne les personnages dans un ordre aléatoire.
func shuffledCharacters() []Character {
	loadOnce.Do(loadData)
	if loadFailure != nil {
		panic("katana: données de jeu invalides: " + loadFailure.Error())
	}

	list := append([]Character(nil), characters...)
	rand.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
	return list
}

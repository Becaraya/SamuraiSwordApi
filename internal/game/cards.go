package game

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
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

// Role représente le rôle secret d'un joueur (sauf le Shogun, révélé).
type Role struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Camp  string `json:"camp"`
	Stars int    `json:"stars,omitempty"`
}

const RoleShogun = "shogun"

var (
	loadOnce    sync.Once
	cardDefs    []cardDefinition
	characters  []Character
	roles       []Role
	rolePrep    map[string]map[string]int
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
	var rawRoles struct {
		Roles []struct {
			ID    string `json:"id"`
			Name  string `json:"nom"`
			Camp  string `json:"camp"`
			Stars int    `json:"etoiles"`
		} `json:"roles"`
		Setup map[string]struct {
			Roles map[string]int `json:"roles"`
		} `json:"preparation"`
	}

	for file, target := range map[string]any{
		"data/cards.json":      &rawCards,
		"data/characters.json": &rawCharacters,
		"data/roles.json":      &rawRoles,
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
	for _, r := range rawRoles.Roles {
		roles = append(roles, Role{ID: r.ID, Name: r.Name, Camp: r.Camp, Stars: r.Stars})
	}
	rolePrep = make(map[string]map[string]int, len(rawRoles.Setup))
	for count, setup := range rawRoles.Setup {
		rolePrep[count] = setup.Roles
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

// shuffledRoles retourne les rôles de la partie (selon le nombre de joueurs) dans un ordre aléatoire.
func shuffledRoles(playerCount int) []Role {
	loadOnce.Do(loadData)
	if loadFailure != nil {
		panic("katana: données de jeu invalides: " + loadFailure.Error())
	}

	var list []Role
	for kind, count := range rolePrep[strconv.Itoa(playerCount)] {
		// Les ninjas se distinguent par leur nombre de shurikens (ninja_1, ninja_2...).
		var pool []Role
		for _, r := range roles {
			if r.ID == kind || strings.HasPrefix(r.ID, kind+"_") {
				pool = append(pool, r)
			}
		}
		rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		// Un rôle manquant (ex. 2 samouraïs pour 1 carte) est dupliqué.
		for i := 0; i < count; i++ {
			list = append(list, pool[i%len(pool)])
		}
	}
	rand.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
	return list
}

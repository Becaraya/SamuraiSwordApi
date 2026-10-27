package assets

import "embed"

// Data contient les définitions JSON du jeu (cartes, personnages, rôles).
//
//go:embed data/*.json
var Data embed.FS

// Package data exposes embedded built-in YAML definitions.
// Embed directives must live here (next to the data subdirectories)
// because Go does not allow "../" in //go:embed paths.
package data

import "embed"

//go:embed all:modules
var ModulesFS embed.FS

//go:embed all:effects
var EffectsFS embed.FS

//go:embed all:boards
var BoardsFS embed.FS

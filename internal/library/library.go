// Package library loads built-in module, effect, and board YAML definitions
// from the embedded data/ directory.
package library

import (
	"fmt"
	"io/fs"
	"path"

	"github.com/karthangar/matteresp32hub/data"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// LoadModules parses and returns all built-in module definitions.
func LoadModules() ([]*yamldef.Module, error) {
	return loadDir(data.ModulesFS, "modules", yamldef.ParseModule)
}

// LoadEffects parses and returns all built-in effect definitions.
func LoadEffects() ([]*yamldef.Effect, error) {
	return loadDir(data.EffectsFS, "effects", yamldef.ParseEffect)
}

// LoadBoards parses and returns all built-in board profile definitions.
func LoadBoards() ([]*yamldef.BoardProfile, error) {
	return loadDir(data.BoardsFS, "boards", yamldef.ParseBoard)
}

func loadDir[T any](fsys fs.ReadDirFS, dir string, parse func([]byte) (*T, error)) ([]*T, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read embedded dir %q: %w", dir, err)
	}
	var results []*T
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		v, err := parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		results = append(results, v)
	}
	return results, nil
}

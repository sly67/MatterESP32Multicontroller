// Package seed populates the database with built-in module and effect
// definitions on first boot. All operations are idempotent (INSERT OR IGNORE).
package seed

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/library"
)

// SeedBuiltins loads built-in modules and effects from the embedded library
// and inserts them into the database. Safe to call on every startup.
func SeedBuiltins(database *db.Database) error {
	mods, err := library.LoadModules()
	if err != nil {
		return fmt.Errorf("load built-in modules: %w", err)
	}
	for _, m := range mods {
		body, err := yaml.Marshal(m)
		if err != nil {
			return fmt.Errorf("marshal module %q: %w", m.ID, err)
		}
		if err := database.CreateModule(db.ModuleRow{
			ID:       m.ID,
			Name:     m.Name,
			Category: m.Category,
			Builtin:  true,
			YAMLBody: string(body),
		}); err != nil {
			return fmt.Errorf("seed module %q: %w", m.ID, err)
		}
	}

	effs, err := library.LoadEffects()
	if err != nil {
		return fmt.Errorf("load built-in effects: %w", err)
	}
	for _, e := range effs {
		body, err := yaml.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal effect %q: %w", e.ID, err)
		}
		if err := database.CreateEffect(db.EffectRow{
			ID:       e.ID,
			Name:     e.Name,
			Builtin:  true,
			YAMLBody: string(body),
		}); err != nil {
			return fmt.Errorf("seed effect %q: %w", e.ID, err)
		}
	}
	return nil
}

package db

import "time"

// EffectRow is an effect record in the database.
type EffectRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Builtin   bool      `json:"builtin"`
	YAMLBody  string    `json:"yaml_body"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateEffect inserts an effect record. Ignores conflict (idempotent for seeding).
func (d *Database) CreateEffect(e EffectRow) error {
	builtin := 0
	if e.Builtin {
		builtin = 1
	}
	_, err := d.DB.Exec(
		`INSERT OR IGNORE INTO effects (id, name, builtin, yaml_body)
		 VALUES (?, ?, ?, ?)`,
		e.ID, e.Name, builtin, e.YAMLBody)
	return err
}

// GetEffect retrieves an effect by ID.
func (d *Database) GetEffect(id string) (EffectRow, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, builtin, yaml_body, created_at FROM effects WHERE id = ?`, id)
	var e EffectRow
	var builtin int
	if err := row.Scan(&e.ID, &e.Name, &builtin, &e.YAMLBody, &e.CreatedAt); err != nil {
		return EffectRow{}, err
	}
	e.Builtin = builtin == 1
	return e, nil
}

// ListEffects returns all effects ordered by name.
func (d *Database) ListEffects() ([]EffectRow, error) {
	rows, err := d.DB.Query(
		`SELECT id, name, builtin, yaml_body, created_at FROM effects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var effs []EffectRow
	for rows.Next() {
		var e EffectRow
		var builtin int
		if err := rows.Scan(&e.ID, &e.Name, &builtin, &e.YAMLBody, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Builtin = builtin == 1
		effs = append(effs, e)
	}
	return effs, rows.Err()
}

// DeleteEffect removes an effect by ID.
func (d *Database) DeleteEffect(id string) error {
	_, err := d.DB.Exec(`DELETE FROM effects WHERE id = ?`, id)
	return err
}

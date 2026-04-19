package db

import "time"

// ModuleRow is a module record in the database.
type ModuleRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Builtin   bool      `json:"builtin"`
	YAMLBody  string    `json:"yaml_body"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateModule inserts a module record. Ignores conflict (idempotent for user modules).
func (d *Database) CreateModule(m ModuleRow) error {
	builtin := 0
	if m.Builtin {
		builtin = 1
	}
	_, err := d.DB.Exec(
		`INSERT OR IGNORE INTO modules (id, name, category, builtin, yaml_body)
		 VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.Name, m.Category, builtin, m.YAMLBody)
	return err
}

// UpsertBuiltinModule inserts or updates a builtin module so that source changes
// propagate to the DB on every restart. User-created modules with the same ID
// (builtin = 0) are left untouched.
func (d *Database) UpsertBuiltinModule(m ModuleRow) error {
	_, err := d.DB.Exec(
		`INSERT INTO modules (id, name, category, builtin, yaml_body)
		 VALUES (?, ?, ?, 1, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name     = excluded.name,
		   category = excluded.category,
		   yaml_body = excluded.yaml_body
		 WHERE modules.builtin = 1`,
		m.ID, m.Name, m.Category, m.YAMLBody)
	return err
}

// GetModule retrieves a module by ID.
func (d *Database) GetModule(id string) (ModuleRow, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, category, builtin, yaml_body, created_at FROM modules WHERE id = ?`, id)
	var m ModuleRow
	var builtin int
	if err := row.Scan(&m.ID, &m.Name, &m.Category, &builtin, &m.YAMLBody, &m.CreatedAt); err != nil {
		return ModuleRow{}, err
	}
	m.Builtin = builtin == 1
	return m, nil
}

// ListModules returns all modules ordered by name.
func (d *Database) ListModules() ([]ModuleRow, error) {
	rows, err := d.DB.Query(
		`SELECT id, name, category, builtin, yaml_body, created_at FROM modules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mods []ModuleRow
	for rows.Next() {
		var m ModuleRow
		var builtin int
		if err := rows.Scan(&m.ID, &m.Name, &m.Category, &builtin, &m.YAMLBody, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Builtin = builtin == 1
		mods = append(mods, m)
	}
	return mods, rows.Err()
}

// DeleteModule removes a module by ID.
func (d *Database) DeleteModule(id string) error {
	_, err := d.DB.Exec(`DELETE FROM modules WHERE id = ?`, id)
	return err
}

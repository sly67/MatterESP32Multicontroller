package db

import "time"

// TemplateRow is a template record in the database.
type TemplateRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Board     string    `json:"board"`
	YAMLBody  string    `json:"yaml_body"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateTemplate inserts a template record. Ignores conflict (idempotent).
func (d *Database) CreateTemplate(t TemplateRow) error {
	_, err := d.DB.Exec(
		`INSERT OR IGNORE INTO templates (id, name, board, yaml_body)
		 VALUES (?, ?, ?, ?)`,
		t.ID, t.Name, t.Board, t.YAMLBody)
	return err
}

// GetTemplate retrieves a template by ID.
func (d *Database) GetTemplate(id string) (TemplateRow, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, board, yaml_body, created_at FROM templates WHERE id = ?`, id)
	var t TemplateRow
	if err := row.Scan(&t.ID, &t.Name, &t.Board, &t.YAMLBody, &t.CreatedAt); err != nil {
		return TemplateRow{}, err
	}
	return t, nil
}

// ListTemplates returns all templates ordered by id.
func (d *Database) ListTemplates() ([]TemplateRow, error) {
	rows, err := d.DB.Query(
		`SELECT id, name, board, yaml_body, created_at FROM templates ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tpls []TemplateRow
	for rows.Next() {
		var t TemplateRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Board, &t.YAMLBody, &t.CreatedAt); err != nil {
			return nil, err
		}
		tpls = append(tpls, t)
	}
	return tpls, rows.Err()
}

// DeleteTemplate removes a template by ID.
func (d *Database) DeleteTemplate(id string) error {
	_, err := d.DB.Exec(`DELETE FROM templates WHERE id = ?`, id)
	return err
}

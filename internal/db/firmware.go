package db

import "time"

// FirmwareRow is a firmware version record.
type FirmwareRow struct {
	Version   string    `json:"version"`
	Boards    string    `json:"boards"`  // comma-separated board IDs
	Notes     string    `json:"notes"`
	IsLatest  bool      `json:"is_latest"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateFirmware inserts a firmware record. Ignores conflict.
func (d *Database) CreateFirmware(f FirmwareRow) error {
	latest := 0
	if f.IsLatest {
		latest = 1
	}
	_, err := d.DB.Exec(
		`INSERT OR IGNORE INTO firmware (version, boards, notes, is_latest)
		 VALUES (?, ?, ?, ?)`,
		f.Version, f.Boards, f.Notes, latest)
	return err
}

// GetFirmware retrieves a firmware record by version.
func (d *Database) GetFirmware(version string) (FirmwareRow, error) {
	row := d.DB.QueryRow(
		`SELECT version, boards, notes, is_latest, created_at FROM firmware WHERE version = ?`, version)
	var f FirmwareRow
	var latest int
	if err := row.Scan(&f.Version, &f.Boards, &f.Notes, &latest, &f.CreatedAt); err != nil {
		return FirmwareRow{}, err
	}
	f.IsLatest = latest == 1
	return f, nil
}

// GetLatestFirmware returns the firmware row marked is_latest = 1.
func (d *Database) GetLatestFirmware() (FirmwareRow, error) {
	row := d.DB.QueryRow(
		`SELECT version, boards, notes, is_latest, created_at FROM firmware WHERE is_latest = 1 LIMIT 1`)
	var f FirmwareRow
	var latest int
	if err := row.Scan(&f.Version, &f.Boards, &f.Notes, &latest, &f.CreatedAt); err != nil {
		return FirmwareRow{}, err
	}
	f.IsLatest = latest == 1
	return f, nil
}

// ListFirmware returns all firmware versions ordered newest first.
func (d *Database) ListFirmware() ([]FirmwareRow, error) {
	rows, err := d.DB.Query(
		`SELECT version, boards, notes, is_latest, created_at FROM firmware ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fws []FirmwareRow
	for rows.Next() {
		var f FirmwareRow
		var latest int
		if err := rows.Scan(&f.Version, &f.Boards, &f.Notes, &latest, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.IsLatest = latest == 1
		fws = append(fws, f)
	}
	return fws, rows.Err()
}

// SetLatestFirmware marks the given version as latest and clears all others.
func (d *Database) SetLatestFirmware(version string) error {
	tx, err := d.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE firmware SET is_latest = 0`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE firmware SET is_latest = 1 WHERE version = ?`, version); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteFirmware removes a firmware record by version.
func (d *Database) DeleteFirmware(version string) error {
	_, err := d.DB.Exec(`DELETE FROM firmware WHERE version = ?`, version)
	return err
}

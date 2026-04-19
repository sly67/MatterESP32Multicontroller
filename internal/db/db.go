package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Database wraps a sql.DB with app-specific methods.
type Database struct {
	DB *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*Database, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("mkdir db dir: %w", err)
		}
	}
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1) // SQLite is single-writer

	// Apply pragmas FIRST (before DDL)
	if _, err := sqldb.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}
	// Then apply schema
	if _, err := sqldb.Exec(schema); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	for _, up := range []string{
		`ALTER TABLE devices ADD COLUMN matter_discrim  INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE devices ADD COLUMN matter_passcode INTEGER NOT NULL DEFAULT 0`,
	} {
		sqldb.Exec(up) //nolint:errcheck // column may already exist on new installs
	}

	// ESPHome migration: make template_id nullable + add ESPHome columns.
	// Detect by checking whether firmware_type column exists.
	var fwTypeCount int
	sqldb.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='firmware_type'`).Scan(&fwTypeCount) //nolint:errcheck
	if fwTypeCount == 0 {
		stmts := []string{
			`PRAGMA foreign_keys=OFF`,
			`CREATE TABLE devices_v2 (
				id              TEXT PRIMARY KEY,
				name            TEXT NOT NULL,
				template_id     TEXT,
				fw_version      TEXT NOT NULL DEFAULT '',
				psk             BLOB NOT NULL DEFAULT x'',
				status          TEXT NOT NULL DEFAULT 'unknown',
				last_seen       DATETIME,
				ip              TEXT NOT NULL DEFAULT '',
				matter_discrim  INTEGER NOT NULL DEFAULT 0,
				matter_passcode INTEGER NOT NULL DEFAULT 0,
				firmware_type   TEXT NOT NULL DEFAULT 'matter',
				esphome_config  TEXT NOT NULL DEFAULT '',
				esphome_api_key TEXT NOT NULL DEFAULT '',
				created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`INSERT INTO devices_v2 (id, name, template_id, fw_version, psk, status, last_seen, ip, matter_discrim, matter_passcode, created_at)
			 SELECT id, name, template_id, fw_version, psk, status, last_seen, ip, matter_discrim, matter_passcode, created_at FROM devices`,
			`DROP TABLE devices`,
			`ALTER TABLE devices_v2 RENAME TO devices`,
			`CREATE INDEX IF NOT EXISTS idx_devices_name ON devices(name)`,
			`PRAGMA foreign_keys=ON`,
		}
		for _, s := range stmts {
			if _, err := sqldb.Exec(s); err != nil {
				sqldb.Close()
				return nil, fmt.Errorf("ESPHome migration (%q): %w", s, err)
			}
		}
	}

	return &Database{DB: sqldb}, nil
}

// Close closes the underlying database connection.
func (d *Database) Close() error {
	return d.DB.Close()
}

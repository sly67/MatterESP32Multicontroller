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

	if _, err := sqldb.Exec(schema); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if _, err := sqldb.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}
	return &Database{DB: sqldb}, nil
}

// Close closes the underlying database connection.
func (d *Database) Close() error {
	return d.DB.Close()
}

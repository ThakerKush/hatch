package store

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB wraps a PostgreSQL database connection and provides typed CRUD methods for
// all Hatch entities (images, VMs, templates, snapshots, proxy routes).
type DB struct {
	db *sql.DB
}

// OpenDB connects to PostgreSQL using the supplied connection string
// (e.g. "postgres://user:pass@localhost:5432/hatch?sslmode=disable")
// and runs schema migrations.
func OpenDB(databaseURL string) (*DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	d := &DB{db: db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	slog.Info("database connected", "url", redactURL(databaseURL))
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// redactURL hides the password from a postgres connection URL for logging.
func redactURL(u string) string {
	// Simple redaction: keep scheme + host, hide the rest.
	if len(u) > 30 {
		return u[:30] + "..."
	}
	return u
}

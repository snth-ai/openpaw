package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// DB — единая SQLite база синта. Один файл = всё состояние.
type DB struct {
	*sqlx.DB
}

// Open открывает (или создаёт) базу данных синта.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	db, err := sqlx.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// WAL mode + single connection для write, multiple для read
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	d := &DB{db}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return d, nil
}

func (d *DB) migrate() error {
	_, err := d.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id          TEXT PRIMARY KEY,
			messages    TEXT NOT NULL DEFAULT '[]',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS memories (
			id           TEXT PRIMARY KEY,
			text         TEXT NOT NULL,
			category     TEXT NOT NULL DEFAULT 'fact',
			content_type TEXT NOT NULL DEFAULT 'text',
			scope        TEXT NOT NULL DEFAULT 'default',
			importance   REAL NOT NULL DEFAULT 0.5,
			embedding    BLOB,
			access_count INTEGER NOT NULL DEFAULT 0,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_access  DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);
		CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
		CREATE INDEX IF NOT EXISTS idx_memories_importance ON memories(importance);

		CREATE TABLE IF NOT EXISTS emotional_state (
			id          TEXT PRIMARY KEY DEFAULT 'default',
			state_json  TEXT NOT NULL,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS personality (
			id          TEXT PRIMARY KEY DEFAULT 'default',
			matrix_json TEXT NOT NULL,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS scheduled_tasks (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			message    TEXT NOT NULL,
			session_id TEXT NOT NULL,
			interval_ms INTEGER NOT NULL,
			repeat     BOOLEAN NOT NULL DEFAULT 0,
			next_run   DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

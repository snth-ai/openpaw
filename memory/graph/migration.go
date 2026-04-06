package graph

import "github.com/jmoiron/sqlx"

// Migrate создаёт таблицы графа в существующей SQLite базе.
func Migrate(db *sqlx.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS graph_nodes (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL,
			type            TEXT NOT NULL DEFAULT 'entity',
			aliases         TEXT DEFAULT '[]',
			tags            TEXT DEFAULT '[]',
			summary         TEXT DEFAULT '',
			properties      TEXT DEFAULT '{}',
			embedding       BLOB,
			mention_count   INTEGER DEFAULT 1,
			created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS graph_edges (
			id              TEXT PRIMARY KEY,
			from_id         TEXT NOT NULL,
			to_id           TEXT NOT NULL,
			relation        TEXT NOT NULL,
			relation_group  TEXT NOT NULL DEFAULT 'general',
			strength        REAL NOT NULL DEFAULT 0.8,
			valid_from      DATETIME,
			valid_until     DATETIME,
			created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
			invalidated_at  DATETIME,
			invalidated_by  TEXT,
			context         TEXT DEFAULT '',
			source_episode  TEXT,
			properties      TEXT DEFAULT '{}',
			FOREIGN KEY (from_id) REFERENCES graph_nodes(id),
			FOREIGN KEY (to_id) REFERENCES graph_nodes(id)
		);

		CREATE TABLE IF NOT EXISTS graph_episodes (
			id              TEXT PRIMARY KEY,
			summary         TEXT NOT NULL,
			timestamp       DATETIME NOT NULL DEFAULT (datetime('now')),
			node_ids        TEXT DEFAULT '[]',
			edge_ids        TEXT DEFAULT '[]'
		);

		CREATE INDEX IF NOT EXISTS idx_graph_edges_from_active
			ON graph_edges(from_id) WHERE invalidated_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_graph_edges_to_active
			ON graph_edges(to_id) WHERE invalidated_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_graph_edges_group
			ON graph_edges(relation_group) WHERE invalidated_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_graph_nodes_name
			ON graph_nodes(name COLLATE NOCASE);
		CREATE INDEX IF NOT EXISTS idx_graph_nodes_type
			ON graph_nodes(type);
		CREATE INDEX IF NOT EXISTS idx_graph_nodes_importance
			ON graph_nodes(mention_count DESC);
	`)
	if err != nil {
		return err
	}

	// Migration: add min_strength column if not exists
	db.Exec(`ALTER TABLE graph_edges ADD COLUMN min_strength REAL NOT NULL DEFAULT 0.0`)

	return nil
}

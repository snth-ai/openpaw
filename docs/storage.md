# Storage — SQLite Unified Database

## Overview

Single SQLite file per synth = all persistent state. Sessions, memories, and future emotional state / personality all live here. Pure Go driver (`modernc.org/sqlite`), no CGO.

## Database: `data/synth.db`

Path configurable via `DB_PATH` env var (default: `./data/synth.db`).

Opens with WAL journal mode + 5s busy timeout for concurrent reads.

## Package: `storage/`

### db.go
- `Open(path) → *DB` — opens/creates database, runs migrations
- Auto-creates directory structure
- Single write connection (SQLite limitation), multiple read
- Migrations create all tables + indices on first run

### Tables

#### sessions
```sql
id          TEXT PRIMARY KEY     -- session identifier
messages    TEXT NOT NULL        -- JSON array of llm.Message
created_at  DATETIME
updated_at  DATETIME
```

#### memories
```sql
id           TEXT PRIMARY KEY
text         TEXT NOT NULL
category     TEXT NOT NULL       -- preference/fact/decision/entity/reflection
content_type TEXT NOT NULL       -- text/image/audio/video
scope        TEXT NOT NULL       -- "default" or named scope
importance   REAL NOT NULL
embedding    BLOB               -- float32 array as little-endian bytes (768 dims * 4 bytes = 3072 bytes)
access_count INTEGER NOT NULL
created_at   DATETIME
updated_at   DATETIME
last_access  DATETIME
```

Indices: category, scope, importance.

### sessions.go — SessionStore
- `Load(id) → []llm.Message` — load session history
- `Save(id, msgs)` — full replace (upsert)
- `Append(id, msgs...)` — load + append + save
- `List() → []string` — all session IDs sorted by updated_at

### memories.go — MemoryStore (implements memory.Store interface)
- Same interface as old FileStore, but backed by SQLite
- Embeddings stored as BLOB (binary little-endian float32)
- Vector search: brute-force cosine distance (loads all, computes in Go)
- `encodeEmbedding([]float32) → []byte` / `decodeEmbedding([]byte) → []float32`

## Migration from FileStore

Old `memory.FileStore` (JSON) and `session.Store` (in-memory) are replaced by:
- `storage.MemoryStore` — same `memory.Store` interface
- `storage.SessionStore` — persistent sessions

Old packages `session/` and `memory/store.go` FileStore are no longer used in main.go.

## LanceDB Vector Store

For vector search, LanceDB is used alongside SQLite. SQLite handles structured state, LanceDB handles embeddings.

### storage/lancedb.go — LanceStore (implements memory.Store)
- Uses `github.com/lancedb/lancedb-go` (v0.1.2, CGO + pre-built Rust native lib)
- Table `memories` with 768-dim float32 vector column
- Native vector search (cosine similarity with indices)
- Arrow-based record format for insert
- Auto-fallback: if LanceDB fails to connect, SQLite MemoryStore is used

### Build requirements
LanceDB requires CGO and native library. Use Makefile:
```bash
make build   # sets CGO_CFLAGS and CGO_LDFLAGS automatically
make run     # runs with CGO flags
```

Native lib location: `lib/darwin_arm64/liblancedb_go.a` (downloaded via download-artifacts.sh)

### Data path
- `LANCEDB_PATH` env var (default: `./data/memory.lance`)

## Graph Memory Tables (SQLite)

Added to the same `synth.db`:

**graph_nodes** — entities in knowledge graph
```sql
id, name, type, aliases (JSON), tags (JSON), summary, properties (JSON),
embedding (BLOB 768-dim), mention_count, created_at, updated_at
```

**graph_edges** — relations (bi-temporal)
```sql
id, from_id, to_id, relation, relation_group, strength,
valid_from, valid_until, created_at, invalidated_at, invalidated_by,
context, source_episode, properties (JSON)
```

**graph_episodes** — dialogue events
```sql
id, summary, timestamp, node_ids (JSON), edge_ids (JSON)
```

Indices: active edges by from/to, relation_group, node name (case-insensitive), mention_count.

## What's NOT Implemented Yet

- Full-text search via LanceDB FTS or SQLite FTS5
- LanceDB index creation (IVF, HNSW) for large-scale search
- Graph visualization export with full edge data

## ENV

- `DB_PATH` — path to SQLite file (default: `./data/synth.db`)
- `LANCEDB_PATH` — path to LanceDB directory (default: `./data/memory.lance`)
- `MEMORY_LOOP_MODEL` — cheap model for graph retrieval (default: `google/gemini-3.1-flash-lite-preview`)

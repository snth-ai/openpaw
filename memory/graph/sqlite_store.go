package graph

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/openpaw/server/uid"
)

// SQLiteStore — SQLite реализация GraphStore.
type SQLiteStore struct {
	db *sqlx.DB
}

func NewSQLiteStore(db *sqlx.DB) (*SQLiteStore, error) {
	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("graph migrate: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// ═══════════════════════════════════════════
// NODES
// ═══════════════════════════════════════════

func (s *SQLiteStore) GetNode(id string) (*Node, error) {
	row := s.db.QueryRow("SELECT * FROM graph_nodes WHERE id = ?", id)
	return scanNode(row)
}

func (s *SQLiteStore) GetNodeByName(name string) (*Node, error) {
	// Exact match by name
	row := s.db.QueryRow("SELECT * FROM graph_nodes WHERE name COLLATE NOCASE = ?", name)
	node, err := scanNode(row)
	if err == nil {
		return node, nil
	}

	// Search in aliases
	rows, err := s.db.Query("SELECT * FROM graph_nodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nameLower := strings.ToLower(name)
	for rows.Next() {
		n, err := scanNodeFromRows(rows)
		if err != nil {
			continue
		}
		for _, alias := range n.Aliases {
			if strings.ToLower(alias) == nameLower {
				return n, nil
			}
		}
	}
	return nil, fmt.Errorf("node %q not found", name)
}

func (s *SQLiteStore) FindNodesByNames(names []string) ([]*Node, error) {
	if len(names) == 0 {
		return nil, nil
	}

	// Load all nodes and match against names and aliases
	rows, err := s.db.Query("SELECT * FROM graph_nodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[strings.ToLower(n)] = true
	}

	seen := make(map[string]bool)
	var result []*Node

	for rows.Next() {
		node, err := scanNodeFromRows(rows)
		if err != nil {
			continue
		}
		if seen[node.ID] {
			continue
		}

		// Check name
		if nameSet[strings.ToLower(node.Name)] {
			seen[node.ID] = true
			result = append(result, node)
			continue
		}

		// Check aliases
		for _, alias := range node.Aliases {
			if nameSet[strings.ToLower(alias)] {
				seen[node.ID] = true
				result = append(result, node)
				break
			}
		}
	}
	return result, nil
}

func (s *SQLiteStore) FindNodesByEmbedding(emb []float32, threshold float64, limit int) ([]*Node, error) {
	rows, err := s.db.Query("SELECT * FROM graph_nodes WHERE embedding IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		node *Node
		sim  float64
	}
	var results []scored

	for rows.Next() {
		node, err := scanNodeFromRows(rows)
		if err != nil {
			continue
		}
		if len(node.Embedding) == 0 {
			continue
		}
		sim := cosineSimilarity(emb, node.Embedding)
		if sim >= threshold {
			results = append(results, scored{node, sim})
		}
	}

	// Sort by similarity desc
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].sim > results[j-1].sim; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	var out []*Node
	for i, r := range results {
		if i >= limit {
			break
		}
		out = append(out, r.node)
	}
	return out, nil
}

func (s *SQLiteStore) UpsertNode(node *Node) error {
	if node.ID == "" {
		node.ID = uid.New()
	}
	now := time.Now()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	node.UpdatedAt = now

	aliasesJSON, _ := json.Marshal(node.Aliases)
	tagsJSON, _ := json.Marshal(node.Tags)
	propsJSON, _ := json.Marshal(node.Properties)

	_, err := s.db.Exec(`
		INSERT INTO graph_nodes (id, name, type, aliases, tags, summary, properties, embedding, mention_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, type=excluded.type, aliases=excluded.aliases, tags=excluded.tags,
			summary=excluded.summary, properties=excluded.properties, embedding=excluded.embedding,
			mention_count=excluded.mention_count, updated_at=excluded.updated_at
	`, node.ID, node.Name, node.Type, string(aliasesJSON), string(tagsJSON),
		node.Summary, string(propsJSON), encodeEmbedding(node.Embedding),
		node.MentionCount, node.CreatedAt.Format(time.RFC3339), node.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) DeleteNode(id string) error {
	// Удаляем все edges связанные с нодой
	s.db.Exec("DELETE FROM graph_edges WHERE from_id = ? OR to_id = ?", id, id)
	_, err := s.db.Exec("DELETE FROM graph_nodes WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) IncrementMentionCount(id string) error {
	_, err := s.db.Exec("UPDATE graph_nodes SET mention_count = mention_count + 1, updated_at = datetime('now') WHERE id = ?", id)
	return err
}

// ═══════════════════════════════════════════
// EDGES
// ═══════════════════════════════════════════

func (s *SQLiteStore) AddEdge(edge *Edge) error {
	if edge.ID == "" {
		edge.ID = uid.New()
	}
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = time.Now()
	}
	propsJSON, _ := json.Marshal(edge.Properties)

	_, err := s.db.Exec(`
		INSERT INTO graph_edges (id, from_id, to_id, relation, relation_group, strength, min_strength,
			valid_from, valid_until, created_at, context, source_episode, properties)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, edge.ID, edge.FromID, edge.ToID, edge.Relation, edge.RelationGroup, edge.Strength, edge.MinStrength,
		timePtr(edge.ValidFrom), timePtr(edge.ValidUntil),
		edge.CreatedAt.Format(time.RFC3339), edge.Context, edge.SourceEpisode, string(propsJSON))
	return err
}

func (s *SQLiteStore) UpdateEdge(edge *Edge) error {
	_, err := s.db.Exec(`
		UPDATE graph_edges SET strength=?, context=?, properties=? WHERE id=?
	`, edge.Strength, edge.Context, mustJSON(edge.Properties), edge.ID)
	return err
}

func (s *SQLiteStore) GetActiveEdgesForNode(nodeID string) ([]Edge, error) {
	return s.queryEdges(
		"SELECT " + edgeCols + " FROM graph_edges WHERE (from_id=? OR to_id=?) AND invalidated_at IS NULL ORDER BY strength DESC",
		nodeID, nodeID)
}

func (s *SQLiteStore) GetAllEdgesForNode(nodeID string) ([]Edge, error) {
	return s.queryEdges(
		"SELECT " + edgeCols + " FROM graph_edges WHERE (from_id=? OR to_id=?) ORDER BY created_at DESC",
		nodeID, nodeID)
}

func (s *SQLiteStore) GetActiveEdgesByGroup(nodeID string, group string) ([]Edge, error) {
	return s.queryEdges(
		"SELECT " + edgeCols + " FROM graph_edges WHERE (from_id=? OR to_id=?) AND relation_group=? AND invalidated_at IS NULL ORDER BY strength DESC",
		nodeID, nodeID, group)
}

func (s *SQLiteStore) FindDuplicateEdge(fromID, toID, relation string) (*Edge, error) {
	edges, err := s.queryEdges(
		"SELECT " + edgeCols + " FROM graph_edges WHERE from_id=? AND to_id=? AND relation=? AND invalidated_at IS NULL LIMIT 1",
		fromID, toID, relation)
	if err != nil || len(edges) == 0 {
		return nil, err
	}
	return &edges[0], nil
}

func (s *SQLiteStore) InvalidateEdge(id string, replacedBy string, reason string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE graph_edges SET invalidated_at=?, invalidated_by=?, valid_until=?,
			context=CASE WHEN ?!='' THEN context || ' [invalidated: ' || ? || ']' ELSE context END
		WHERE id=? AND invalidated_at IS NULL
	`, now.Format(time.RFC3339), replacedBy, now.Format(time.RFC3339), reason, reason, id)
	return err
}

func (s *SQLiteStore) FindEdgesByContextEmbedding(emb []float32, threshold float64, limit int) ([]Edge, error) {
	// Для context embedding — нужно загрузить все active edges и сравнить
	// Context обычно короткий, embedding по нему — приблизительный
	// TODO: добавить context_embedding column для точного поиска
	// Сейчас: text search как fallback
	return nil, nil
}

// ═══════════════════════════════════════════
// EPISODES
// ═══════════════════════════════════════════

func (s *SQLiteStore) AddEpisode(episode *Episode) error {
	if episode.ID == "" {
		episode.ID = uid.New()
	}
	nodeIDsJSON, _ := json.Marshal(episode.NodeIDs)
	edgeIDsJSON, _ := json.Marshal(episode.EdgeIDs)

	_, err := s.db.Exec(`
		INSERT INTO graph_episodes (id, summary, timestamp, node_ids, edge_ids)
		VALUES (?, ?, ?, ?, ?)
	`, episode.ID, episode.Summary, episode.Timestamp.Format(time.RFC3339),
		string(nodeIDsJSON), string(edgeIDsJSON))
	return err
}

// ═══════════════════════════════════════════
// RETRIEVAL
// ═══════════════════════════════════════════

func (s *SQLiteStore) GetAllNodes() ([]Node, error) {
	rows, err := s.db.Query("SELECT * FROM graph_nodes ORDER BY mention_count DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		node, err := scanNodeFromRows(rows)
		if err != nil {
			continue
		}
		nodes = append(nodes, *node)
	}
	return nodes, nil
}

const edgeCols = "id, from_id, to_id, relation, relation_group, strength, min_strength, valid_from, valid_until, created_at, invalidated_at, invalidated_by, context, source_episode, properties"

func (s *SQLiteStore) GetAllActiveEdges() ([]Edge, error) {
	return s.queryEdges("SELECT " + edgeCols + " FROM graph_edges WHERE invalidated_at IS NULL ORDER BY strength DESC")
}

func (s *SQLiteStore) GetSubgraph(nodeIDs []string, includeInvalidated bool) (*Subgraph, error) {
	if len(nodeIDs) == 0 {
		return &Subgraph{}, nil
	}

	sg := &Subgraph{}
	seen := make(map[string]bool)

	for _, nodeID := range nodeIDs {
		node, err := s.GetNode(nodeID)
		if err != nil {
			continue
		}
		if !seen[node.ID] {
			seen[node.ID] = true
			sg.Nodes = append(sg.Nodes, *node)
		}

		var edges []Edge
		if includeInvalidated {
			edges, _ = s.GetAllEdgesForNode(nodeID)
		} else {
			edges, _ = s.GetActiveEdgesForNode(nodeID)
		}

		for _, edge := range edges {
			sg.Edges = append(sg.Edges, edge)

			// Add connected nodes
			otherID := edge.ToID
			if otherID == nodeID {
				otherID = edge.FromID
			}
			if !seen[otherID] {
				seen[otherID] = true
				other, err := s.GetNode(otherID)
				if err == nil {
					sg.Nodes = append(sg.Nodes, *other)
				}
			}
		}
	}

	return sg, nil
}

func (s *SQLiteStore) GetUserProfile(userNodeID string, limit int) (*Subgraph, error) {
	return s.GetSubgraph([]string{userNodeID}, false)
}

// ═══════════════════════════════════════════
// MAINTENANCE
// ═══════════════════════════════════════════

func (s *SQLiteStore) DecayEdgeStrengths(factor float64) (int, error) {
	// Active edges: strength = max(strength * factor, min_strength)
	res1, err := s.db.Exec(`
		UPDATE graph_edges SET strength = MAX(strength * ?, min_strength)
		WHERE invalidated_at IS NULL AND strength > min_strength
	`, factor)
	if err != nil {
		return 0, err
	}
	n1, _ := res1.RowsAffected()

	// Invalidated edges: ускоренный decay (0.95/день), min_strength игнорируется
	res2, err := s.db.Exec(`
		UPDATE graph_edges SET strength = strength * 0.95
		WHERE invalidated_at IS NOT NULL AND strength > 0
	`)
	if err != nil {
		return int(n1), err
	}
	n2, _ := res2.RowsAffected()

	return int(n1) + int(n2), nil
}

func (s *SQLiteStore) PruneWeakInvalidatedEdges(minStrength float64) (int, error) {
	res, err := s.db.Exec(`
		DELETE FROM graph_edges WHERE invalidated_at IS NOT NULL AND strength < ?
	`, minStrength)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) PruneOrphanNodes(minMentions int) (int, error) {
	res, err := s.db.Exec(`
		DELETE FROM graph_nodes WHERE mention_count < ? AND id NOT IN (
			SELECT DISTINCT from_id FROM graph_edges WHERE invalidated_at IS NULL
			UNION
			SELECT DISTINCT to_id FROM graph_edges WHERE invalidated_at IS NULL
		)
	`, minMentions)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) GetStats() (*GraphStats, error) {
	var stats GraphStats
	s.db.Get(&stats.TotalNodes, "SELECT COUNT(*) FROM graph_nodes")
	s.db.Get(&stats.TotalEdges, "SELECT COUNT(*) FROM graph_edges")
	s.db.Get(&stats.ActiveEdges, "SELECT COUNT(*) FROM graph_edges WHERE invalidated_at IS NULL")
	s.db.Get(&stats.InvalidatedEdges, "SELECT COUNT(*) FROM graph_edges WHERE invalidated_at IS NOT NULL")
	s.db.Get(&stats.TotalEpisodes, "SELECT COUNT(*) FROM graph_episodes")
	return &stats, nil
}

func (s *SQLiteStore) Migrate() error {
	return Migrate(s.db)
}

// ═══════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════

func (s *SQLiteStore) queryEdges(query string, args ...any) ([]Edge, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		edge, err := scanEdgeFromRows(rows)
		if err != nil {
			continue
		}
		edges = append(edges, *edge)
	}
	return edges, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanNode(row scannable) (*Node, error) {
	var n Node
	var aliasesStr, tagsStr, propsStr string
	var embBlob []byte
	var createdAt, updatedAt string

	err := row.Scan(&n.ID, &n.Name, &n.Type, &aliasesStr, &tagsStr,
		&n.Summary, &propsStr, &embBlob, &n.MentionCount, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(aliasesStr), &n.Aliases)
	json.Unmarshal([]byte(tagsStr), &n.Tags)
	json.Unmarshal([]byte(propsStr), &n.Properties)
	n.Embedding = decodeEmbedding(embBlob)
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if n.Aliases == nil {
		n.Aliases = []string{}
	}
	if n.Tags == nil {
		n.Tags = []string{}
	}
	if n.Properties == nil {
		n.Properties = map[string]any{}
	}
	return &n, nil
}

func scanNodeFromRows(rows interface{ Scan(...any) error }) (*Node, error) {
	return scanNode(rows)
}

func scanEdgeFromRows(rows interface{ Scan(...any) error }) (*Edge, error) {
	var e Edge
	var propsStr sql.NullString
	var validFrom, validUntil, createdAt, invalidatedAt sql.NullString
	var invalidatedBy, context, sourceEpisode sql.NullString

	err := rows.Scan(&e.ID, &e.FromID, &e.ToID, &e.Relation, &e.RelationGroup,
		&e.Strength, &e.MinStrength, &validFrom, &validUntil, &createdAt, &invalidatedAt,
		&invalidatedBy, &context, &sourceEpisode, &propsStr)
	if err != nil {
		return nil, err
	}

	e.InvalidatedBy = invalidatedBy.String
	e.Context = context.String
	e.SourceEpisode = sourceEpisode.String

	if propsStr.Valid {
		json.Unmarshal([]byte(propsStr.String), &e.Properties)
	}
	if validFrom.Valid {
		t, _ := time.Parse(time.RFC3339, validFrom.String)
		e.ValidFrom = &t
	}
	if validUntil.Valid {
		t, _ := time.Parse(time.RFC3339, validUntil.String)
		e.ValidUntil = &t
	}
	if createdAt.Valid {
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if invalidatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, invalidatedAt.String)
		e.InvalidatedAt = &t
	}
	if e.Properties == nil {
		e.Properties = map[string]any{}
	}
	return &e, nil
}

func encodeEmbedding(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeEmbedding(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

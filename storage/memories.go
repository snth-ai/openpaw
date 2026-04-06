package storage

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/openpaw/server/memory"
	"github.com/openpaw/server/uid"
)

// MemoryStore — SQLite реализация memory.Store.
type MemoryStore struct {
	db *DB
}

func NewMemoryStore(db *DB) *MemoryStore {
	return &MemoryStore{db: db}
}

// memoryRow — строка из БД.
type memoryRow struct {
	ID          string  `db:"id"`
	Text        string  `db:"text"`
	Category    string  `db:"category"`
	ContentType string  `db:"content_type"`
	Scope       string  `db:"scope"`
	Importance  float64 `db:"importance"`
	Embedding   []byte  `db:"embedding"`
	AccessCount int     `db:"access_count"`
	CreatedAt   string  `db:"created_at"`
	UpdatedAt   string  `db:"updated_at"`
	LastAccess  string  `db:"last_access"`
}

func (r *memoryRow) toMemory() memory.Memory {
	m := memory.Memory{
		ID:          r.ID,
		Text:        r.Text,
		Category:    memory.Category(r.Category),
		ContentType: memory.ContentType(r.ContentType),
		Scope:       r.Scope,
		Importance:  r.Importance,
		Embedding:   decodeEmbedding(r.Embedding),
		AccessCount: r.AccessCount,
	}
	m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", r.CreatedAt)
	m.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", r.UpdatedAt)
	m.LastAccess, _ = time.Parse("2006-01-02 15:04:05", r.LastAccess)
	return m
}

func (s *MemoryStore) Add(m *memory.Memory) error {
	if m.ID == "" {
		m.ID = uid.New()
	}
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	m.LastAccess = now
	if m.ContentType == "" {
		m.ContentType = memory.ContentText
	}
	if m.Scope == "" {
		m.Scope = "default"
	}

	_, err := s.db.Exec(`
		INSERT INTO memories (id, text, category, content_type, scope, importance, embedding, access_count, created_at, updated_at, last_access)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, m.ID, m.Text, string(m.Category), string(m.ContentType), m.Scope,
		m.Importance, encodeEmbedding(m.Embedding), m.AccessCount,
		m.CreatedAt.Format("2006-01-02 15:04:05"),
		m.UpdatedAt.Format("2006-01-02 15:04:05"),
		m.LastAccess.Format("2006-01-02 15:04:05"))
	return err
}

func (s *MemoryStore) Search(embedding []float32, limit int, scope string) ([]memory.SearchResult, error) {
	query := "SELECT * FROM memories"
	var args []any
	if scope != "" {
		query += " WHERE scope = ?"
		args = append(args, scope)
	}

	var rows []memoryRow
	if err := s.db.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("select memories: %w", err)
	}

	// Brute-force cosine search (same as FileStore, LanceDB replaces this later)
	var results []memory.SearchResult
	for _, row := range rows {
		m := row.toMemory()
		if len(m.Embedding) == 0 {
			continue
		}
		dist := cosineDistance(embedding, m.Embedding)
		results = append(results, memory.SearchResult{
			Memory:   m,
			Distance: dist,
		})
	}

	// Sort by distance
	sortByDistance(results)

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}
	return results, nil
}

func (s *MemoryStore) Get(id string) (*memory.Memory, error) {
	var row memoryRow
	if err := s.db.Get(&row, "SELECT * FROM memories WHERE id = ?", id); err != nil {
		return nil, fmt.Errorf("memory %q not found", id)
	}
	m := row.toMemory()
	return &m, nil
}

func (s *MemoryStore) Update(id string, text string, meta map[string]any) error {
	if text != "" {
		if _, err := s.db.Exec("UPDATE memories SET text = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", text, id); err != nil {
			return err
		}
	}
	if v, ok := meta["category"]; ok {
		if _, err := s.db.Exec("UPDATE memories SET category = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", fmt.Sprint(v), id); err != nil {
			return err
		}
	}
	if v, ok := meta["importance"]; ok {
		if _, err := s.db.Exec("UPDATE memories SET importance = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", v, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *MemoryStore) Delete(id string) error {
	res, err := s.db.Exec("DELETE FROM memories WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("memory %q not found", id)
	}
	return nil
}

func (s *MemoryStore) DeleteByQuery(embedding []float32, threshold float32, scope string) (int, error) {
	// Загружаем все, ищем похожие, удаляем
	results, err := s.Search(embedding, 0, scope)
	if err != nil {
		return 0, err
	}

	var deleted int
	for _, r := range results {
		if r.Distance < threshold {
			if err := s.Delete(r.ID); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

func (s *MemoryStore) RunDecay(cfg memory.DecayConfig) (int, error) {
	var rows []memoryRow
	if err := s.db.Select(&rows, "SELECT * FROM memories"); err != nil {
		return 0, err
	}

	var deleted int
	for _, row := range rows {
		m := row.toMemory()
		memory.ApplyDecay(&m, cfg)
		if m.Importance <= 0 {
			s.db.Exec("DELETE FROM memories WHERE id = ?", m.ID)
			deleted++
		} else {
			s.db.Exec("UPDATE memories SET importance = ?, updated_at = ? WHERE id = ?",
				m.Importance, time.Now().Format("2006-01-02 15:04:05"), m.ID)
		}
	}
	return deleted, nil
}

func (s *MemoryStore) All(scope string) ([]memory.Memory, error) {
	query := "SELECT * FROM memories"
	var args []any
	if scope != "" {
		query += " WHERE scope = ?"
		args = append(args, scope)
	}

	var rows []memoryRow
	if err := s.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}

	out := make([]memory.Memory, len(rows))
	for i, row := range rows {
		out[i] = row.toMemory()
	}
	return out, nil
}

// encodeEmbedding — []float32 → []byte (little-endian).
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

// decodeEmbedding — []byte → []float32.
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

// cosineDistance = 1 - cosineSimilarity.
func cosineDistance(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 1.0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 1.0
	}
	return float32(1.0 - dot/denom)
}

func sortByDistance(results []memory.SearchResult) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Distance < results[j-1].Distance; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}

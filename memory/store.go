package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openpaw/server/uid"
)

// Store — интерфейс хранилища памяти.
// MVP: JSON файл + brute-force cosine. Потом: LanceDB.
type Store interface {
	Add(m *Memory) error
	Search(embedding []float32, limit int, scope string) ([]SearchResult, error)
	Get(id string) (*Memory, error)
	Update(id string, text string, meta map[string]any) error
	Delete(id string) error
	DeleteByQuery(embedding []float32, threshold float32, scope string) (int, error)
	RunDecay(cfg DecayConfig) (deleted int, err error)
	All(scope string) ([]Memory, error)
}

// FileStore — JSON-файловая реализация Store.
// Достаточно для MVP (сотни-тысячи записей на синта).
type FileStore struct {
	mu       sync.RWMutex
	path     string
	memories map[string]Memory
}

func NewFileStore(dataDir string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Dir(dataDir), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	fs := &FileStore{
		path:     dataDir,
		memories: make(map[string]Memory),
	}

	// Загружаем существующие данные
	data, err := os.ReadFile(dataDir)
	if err == nil {
		var mems []Memory
		if err := json.Unmarshal(data, &mems); err == nil {
			for _, m := range mems {
				fs.memories[m.ID] = m
			}
		}
	}

	return fs, nil
}

func (fs *FileStore) Add(m *Memory) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

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
		m.ContentType = ContentText
	}
	if m.Scope == "" {
		m.Scope = "default"
	}

	fs.memories[m.ID] = *m
	return fs.save()
}

func (fs *FileStore) Search(embedding []float32, limit int, scope string) ([]SearchResult, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var results []SearchResult

	for _, m := range fs.memories {
		if scope != "" && m.Scope != scope {
			continue
		}
		if len(m.Embedding) == 0 {
			continue
		}

		dist := cosineDistance(embedding, m.Embedding)
		results = append(results, SearchResult{
			Memory:   m,
			Distance: dist,
		})
	}

	// Сортируем по distance (меньше = ближе = лучше)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}

	return results, nil
}

func (fs *FileStore) Get(id string) (*Memory, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	m, ok := fs.memories[id]
	if !ok {
		return nil, fmt.Errorf("memory %q not found", id)
	}
	return &m, nil
}

func (fs *FileStore) Update(id string, text string, meta map[string]any) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	m, ok := fs.memories[id]
	if !ok {
		return fmt.Errorf("memory %q not found", id)
	}

	if text != "" {
		m.Text = text
	}
	if v, ok := meta["category"]; ok {
		m.Category = Category(fmt.Sprint(v))
	}
	if v, ok := meta["importance"]; ok {
		if f, ok := v.(float64); ok {
			m.Importance = f
		}
	}
	m.UpdatedAt = time.Now()

	fs.memories[id] = m
	return fs.save()
}

func (fs *FileStore) Delete(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, ok := fs.memories[id]; !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	delete(fs.memories, id)
	return fs.save()
}

func (fs *FileStore) DeleteByQuery(embedding []float32, threshold float32, scope string) (int, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	var toDelete []string
	for id, m := range fs.memories {
		if scope != "" && m.Scope != scope {
			continue
		}
		if len(m.Embedding) == 0 {
			continue
		}
		if cosineDistance(embedding, m.Embedding) < threshold {
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(fs.memories, id)
	}

	if len(toDelete) > 0 {
		if err := fs.save(); err != nil {
			return 0, err
		}
	}

	return len(toDelete), nil
}

func (fs *FileStore) RunDecay(cfg DecayConfig) (int, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	var toDelete []string
	for id, m := range fs.memories {
		ApplyDecay(&m, cfg)
		if m.Importance <= 0 {
			toDelete = append(toDelete, id)
		} else {
			fs.memories[id] = m
		}
	}

	for _, id := range toDelete {
		delete(fs.memories, id)
	}

	return len(toDelete), fs.save()
}

func (fs *FileStore) All(scope string) ([]Memory, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var out []Memory
	for _, m := range fs.memories {
		if scope != "" && m.Scope != scope {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (fs *FileStore) save() error {
	mems := make([]Memory, 0, len(fs.memories))
	for _, m := range fs.memories {
		mems = append(mems, m)
	}
	data, err := json.MarshalIndent(mems, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal memories: %w", err)
	}
	return os.WriteFile(fs.path, data, 0644)
}

// cosineDistance = 1 - cosineSimilarity. Меньше = ближе.
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

// TextSearch — простой keyword search по тексту (BM25-like fallback).
func TextSearch(memories []Memory, query string, limit int) []Memory {
	query = strings.ToLower(query)
	words := strings.Fields(query)

	type scored struct {
		Memory
		score int
	}
	var results []scored

	for _, m := range memories {
		text := strings.ToLower(m.Text)
		score := 0
		for _, w := range words {
			score += strings.Count(text, w)
		}
		if score > 0 {
			results = append(results, scored{m, score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	out := make([]Memory, 0, limit)
	for i, r := range results {
		if i >= limit {
			break
		}
		out = append(out, r.Memory)
	}
	return out
}

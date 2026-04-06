//go:build !cgo

package storage

import (
	"context"
	"errors"

	"github.com/openpaw/server/memory"
)

// LanceDBStore is a stub when CGO is not available.
type LanceDBStore struct{}

// ErrLanceDBNotAvailable is returned when trying to use LanceDB without CGO.
var ErrLanceDBNotAvailable = errors.New("lancedb requires CGO; rebuild with CGO_ENABLED=1")

// NewLanceDBStore returns an error when CGO is not available.
func NewLanceDBStore(ctx context.Context, dbPath string) (*LanceDBStore, error) {
	return nil, ErrLanceDBNotAvailable
}

// EmbeddingStore interface stub
func (l *LanceDBStore) Add(mem *memory.Memory) error {
	return ErrLanceDBNotAvailable
}

func (l *LanceDBStore) Search(emb []float32, limit int, scope string) ([]memory.SearchResult, error) {
	return nil, ErrLanceDBNotAvailable
}

func (l *LanceDBStore) Get(id string) (*memory.Memory, error) {
	return nil, ErrLanceDBNotAvailable
}

func (l *LanceDBStore) Update(id string, text string, meta map[string]any) error {
	return ErrLanceDBNotAvailable
}

func (l *LanceDBStore) Delete(id string) error {
	return ErrLanceDBNotAvailable
}

func (l *LanceDBStore) All(scope string) ([]memory.Memory, error) {
	return nil, ErrLanceDBNotAvailable
}

package memory

import (
	"path/filepath"
	"testing"
)

func TestAdd_AssignsUniqueID(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "memories.json")

	fs, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	m1 := &Memory{Text: "hello", Category: CategoryFact}
	if err := fs.Add(m1); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if m1.ID == "" {
		t.Fatal("Add() should assign a non-empty ID")
	}

	m2 := &Memory{Text: "world", Category: CategoryFact}
	if err := fs.Add(m2); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if m2.ID == "" {
		t.Fatal("Add() should assign a non-empty ID")
	}

	if m1.ID == m2.ID {
		t.Errorf("IDs should be unique: got %s for both", m1.ID)
	}
}

func TestGet_RetrievesByID(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "memories.json")

	fs, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	m := &Memory{Text: "the sky is blue", Category: CategoryFact, Importance: 0.8}
	if err := fs.Add(m); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	got, err := fs.Get(m.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Text != m.Text {
		t.Errorf("Get().Text = %q, want %q", got.Text, m.Text)
	}
	if got.Category != m.Category {
		t.Errorf("Get().Category = %q, want %q", got.Category, m.Category)
	}
	if got.Importance != m.Importance {
		t.Errorf("Get().Importance = %v, want %v", got.Importance, m.Importance)
	}

	_, err = fs.Get("nonexistent-id")
	if err == nil {
		t.Fatal("Get() should return error for nonexistent ID")
	}
}

func TestDelete_RemovesFromStore(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "memories.json")

	fs, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	m := &Memory{Text: "to be deleted", Category: CategoryFact}
	if err := fs.Add(m); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if err := fs.Delete(m.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = fs.Get(m.ID)
	if err == nil {
		t.Fatal("Get() should return error after Delete()")
	}
}

func TestUpdate_ModifiesMeta(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "memories.json")

	fs, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	m := &Memory{Text: "test", Category: CategoryFact, Importance: 0.5}
	if err := fs.Add(m); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = fs.Update(m.ID, "test", map[string]any{"importance": 0.9})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := fs.Get(m.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Importance != 0.9 {
		t.Errorf("Importance = %v, want 0.9", got.Importance)
	}
}

func TestAll_ReturnsAll(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "memories.json")

	fs, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	fs.Add(&Memory{Text: "one", Category: CategoryFact, Scope: "default"})
	fs.Add(&Memory{Text: "two", Category: CategoryFact, Scope: "default"})
	fs.Add(&Memory{Text: "three", Category: CategoryFact, Scope: "other"})

	all, err := fs.All("default")
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(all) != 2 {
		t.Errorf("All() returned %d memories, want 2", len(all))
	}
}

func TestCosineDistance_Basic(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		wantDist float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			wantDist: 0.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{0.0, 1.0, 0.0},
			wantDist: 1.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{-1.0, 0.0, 0.0},
			wantDist: 2.0,
		},
		{
			name:     "mismatched dimensions returns 1.0",
			a:        []float32{1.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			wantDist: 1.0,
		},
		{
			name:     "zero vector returns 1.0",
			a:        []float32{0.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			wantDist: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineDistance(tt.a, tt.b)
			if float64(got) != tt.wantDist {
				t.Errorf("cosineDistance() = %v, want %v", got, tt.wantDist)
			}
		})
	}
}

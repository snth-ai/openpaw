package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/openpaw/server/memory"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "synth.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Таймстемпы обязаны переживать раунд-трип через SQLite: колонки объявлены
// DATETIME, драйвер отдаёт их в своём формате — parse-фейл здесь молча
// обнуляет UpdatedAt и превращает decay в удаление всего стора.
func TestMemoryTimestampRoundTrip(t *testing.T) {
	s := NewMemoryStore(openTestDB(t))
	m := &memory.Memory{Text: "fresh fact", Importance: 0.9}
	if err := s.Add(m); err != nil {
		t.Fatalf("add: %v", err)
	}
	got, err := s.Get(m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatalf("UpdatedAt is zero after round-trip")
	}
	if d := time.Since(got.UpdatedAt); d < -time.Minute || d > time.Minute {
		t.Fatalf("UpdatedAt off by %v after round-trip", d)
	}
}

func TestRunDecayPreservesFreshMemory(t *testing.T) {
	s := NewMemoryStore(openTestDB(t))
	m := &memory.Memory{Text: "fresh fact", Importance: 0.9}
	if err := s.Add(m); err != nil {
		t.Fatalf("add: %v", err)
	}
	deleted, err := s.RunDecay(memory.DefaultDecayConfig())
	if err != nil {
		t.Fatalf("decay: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("fresh memory deleted by decay (deleted=%d)", deleted)
	}
	got, err := s.Get(m.ID)
	if err != nil {
		t.Fatalf("memory gone after decay: %v", err)
	}
	if got.Importance < 0.5 {
		t.Fatalf("importance collapsed after one decay tick: %v", got.Importance)
	}
}

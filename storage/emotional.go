package storage

import (
	"encoding/json"
	"fmt"

	"github.com/openpaw/server/emotional"
)

// EmotionalStore — persistence для emotional state.
type EmotionalStore struct {
	db *DB
}

func NewEmotionalStore(db *DB) *EmotionalStore {
	return &EmotionalStore{db: db}
}

// Load загружает emotional state. Если нет — создаёт дефолтный.
func (s *EmotionalStore) Load() (*emotional.State, error) {
	var raw string
	err := s.db.QueryRow("SELECT state_json FROM emotional_state WHERE id = 'default'").Scan(&raw)
	if err != nil {
		// Новый синт — создаём начальный стейт
		state := emotional.NewState()
		if err := s.Save(&state); err != nil {
			return nil, err
		}
		return &state, nil
	}

	var state emotional.State
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, fmt.Errorf("unmarshal emotional state: %w", err)
	}
	return &state, nil
}

// Save сохраняет emotional state.
func (s *EmotionalStore) Save(state *emotional.State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal emotional state: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO emotional_state (id, state_json, updated_at) VALUES ('default', ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET state_json = excluded.state_json, updated_at = CURRENT_TIMESTAMP
	`, string(data))
	return err
}

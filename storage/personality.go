package storage

import (
	"encoding/json"
	"fmt"

	"github.com/openpaw/server/personality"
)

// PersonalityStore — persistence для personality matrix.
type PersonalityStore struct {
	db *DB
}

func NewPersonalityStore(db *DB) *PersonalityStore {
	return &PersonalityStore{db: db}
}

// Load загружает personality matrix. Если нет — генерирует рандомную и сохраняет.
func (s *PersonalityStore) Load() (*personality.Matrix, error) {
	var raw string
	err := s.db.QueryRow("SELECT matrix_json FROM personality WHERE id = 'default'").Scan(&raw)
	if err != nil {
		// Новый синт — генерируем характер
		matrix := personality.Generate()
		if err := s.Save(&matrix); err != nil {
			return nil, err
		}
		return &matrix, nil
	}

	var matrix personality.Matrix
	if err := json.Unmarshal([]byte(raw), &matrix); err != nil {
		return nil, fmt.Errorf("unmarshal personality: %w", err)
	}
	return &matrix, nil
}

// Save сохраняет personality matrix (вызывается только при создании).
func (s *PersonalityStore) Save(matrix *personality.Matrix) error {
	data, err := json.Marshal(matrix)
	if err != nil {
		return fmt.Errorf("marshal personality: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO personality (id, matrix_json) VALUES ('default', ?)
		ON CONFLICT(id) DO UPDATE SET matrix_json = excluded.matrix_json
	`, string(data))
	return err
}

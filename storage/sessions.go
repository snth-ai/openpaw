package storage

import (
	"encoding/json"
	"fmt"

	"github.com/openpaw/server/llm"
)

// SessionStore — persistent sessions через SQLite.
type SessionStore struct {
	db *DB
}

func NewSessionStore(db *DB) *SessionStore {
	return &SessionStore{db: db}
}

// Load загружает историю сообщений сессии.
func (s *SessionStore) Load(id string) ([]llm.Message, error) {
	var raw string
	err := s.db.QueryRow("SELECT messages FROM sessions WHERE id = ?", id).Scan(&raw)
	if err != nil {
		// Новая сессия
		return nil, nil
	}

	var msgs []llm.Message
	if err := json.Unmarshal([]byte(raw), &msgs); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return msgs, nil
}

// Save сохраняет историю сообщений сессии.
func (s *SessionStore) Save(id string, msgs []llm.Message) error {
	data, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO sessions (id, messages, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET messages = excluded.messages, updated_at = CURRENT_TIMESTAMP
	`, id, string(data))
	return err
}

// Append добавляет сообщения к существующей сессии.
func (s *SessionStore) Append(id string, newMsgs ...llm.Message) error {
	msgs, err := s.Load(id)
	if err != nil {
		return err
	}
	msgs = append(msgs, newMsgs...)
	return s.Save(id, msgs)
}

// List возвращает все ID сессий.
func (s *SessionStore) List() ([]string, error) {
	var ids []string
	return ids, s.db.Select(&ids, "SELECT id FROM sessions ORDER BY updated_at DESC")
}

package storage

import (
	"fmt"
	"time"
)

// ScheduledTask — persistent scheduled task.
type ScheduledTask struct {
	ID         string `db:"id"`
	Name       string `db:"name"`
	Message    string `db:"message"`
	SessionID  string `db:"session_id"`
	IntervalMs int64  `db:"interval_ms"`
	Repeat     bool   `db:"repeat"`
	NextRun    string `db:"next_run"`
	CreatedAt  string `db:"created_at"`
}

// TaskStore — persistence для scheduled tasks.
type TaskStore struct {
	db *DB
}

func NewTaskStore(db *DB) *TaskStore {
	return &TaskStore{db: db}
}

// Save сохраняет задачу.
func (s *TaskStore) Save(id, name, message, sessionID string, interval time.Duration, repeat bool, nextRun time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO scheduled_tasks (id, name, message, session_id, interval_ms, repeat, next_run)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET next_run = excluded.next_run
	`, id, name, message, sessionID, interval.Milliseconds(), repeat, nextRun.Format(time.RFC3339))
	return err
}

// Delete удаляет задачу.
func (s *TaskStore) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM scheduled_tasks WHERE id = ?", id)
	return err
}

// LoadAll загружает все задачи.
func (s *TaskStore) LoadAll() ([]ScheduledTask, error) {
	var tasks []ScheduledTask
	err := s.db.Select(&tasks, "SELECT * FROM scheduled_tasks")
	if err != nil {
		return nil, fmt.Errorf("load tasks: %w", err)
	}
	return tasks, nil
}

package session

import (
	"sync"

	"github.com/openpaw/server/llm"
)

// Session хранит историю сообщений одного разговора.
type Session struct {
	mu       sync.Mutex
	messages []llm.Message
}

func New() *Session {
	return &Session{}
}

// Append добавляет сообщения в историю.
func (s *Session) Append(msgs ...llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msgs...)
}

// Messages возвращает копию истории.
func (s *Session) Messages() []llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]llm.Message, len(s.messages))
	copy(out, s.messages)
	return out
}

// Store — потокобезопасное хранилище сессий по ID.
type Store struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]*Session)}
}

// Get возвращает сессию по ID, создаёт новую если нет.
func (s *Store) Get(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		sess = New()
		s.sessions[id] = sess
	}
	return sess
}

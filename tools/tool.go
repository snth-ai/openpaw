package tools

import (
	"encoding/json"
	"sort"
)

// Tool — интерфейс для всех тулов в системе.
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage // JSON Schema
	Execute(args json.RawMessage) (string, error)
}

// ContextAware — тул который знает контекст вызова (session_id и т.д.).
type ContextAware interface {
	SetContext(ctx CallContext)
}

// CallContext — контекст текущего вызова.
type CallContext struct {
	SessionID string
}

// Registry хранит все зарегистрированные тулы.
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// RemoveByType удаляет тулы по предикату.
func (r *Registry) RemoveByType(predicate func(Tool) bool) {
	for name, t := range r.tools {
		if predicate(t) {
			delete(r.tools, name)
		}
	}
}

// SetContext устанавливает контекст для всех ContextAware тулов.
func (r *Registry) SetContext(ctx CallContext) {
	for _, t := range r.tools {
		if ca, ok := t.(ContextAware); ok {
			ca.SetContext(ctx)
		}
	}
}

// ForLLM возвращает описание тулов в формате OpenAI function calling.
// Сортируем по имени для стабильного порядка (важно для prompt caching).
// Последний тул получает cache_control breakpoint.
func (r *Registry) ForLLM() []map[string]any {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]map[string]any, 0, len(names))
	for i, name := range names {
		t := r.tools[name]
		entry := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  json.RawMessage(t.Parameters()),
			},
		}
		if i == len(names)-1 {
			entry["cache_control"] = map[string]any{"type": "ephemeral", "ttl": "1h"}
		}
		out = append(out, entry)
	}
	return out
}

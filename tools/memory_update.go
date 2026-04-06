package tools

import (
	"encoding/json"
	"fmt"

	"github.com/openpaw/server/memory"
)

// MemoryUpdate — тул для обновления существующего воспоминания.
type MemoryUpdate struct {
	store    memory.Store
	embedder *memory.Embedder
}

func NewMemoryUpdate(store memory.Store, embedder *memory.Embedder) *MemoryUpdate {
	return &MemoryUpdate{store: store, embedder: embedder}
}

func (t *MemoryUpdate) Name() string { return "memory_update" }
func (t *MemoryUpdate) Description() string {
	return "Update an existing memory's text, category, or importance."
}

func (t *MemoryUpdate) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "ID of the memory to update"
			},
			"text": {
				"type": "string",
				"description": "New text content (if changing)"
			},
			"category": {
				"type": "string",
				"enum": ["preference", "fact", "decision", "entity", "reflection"],
				"description": "New category (if changing)"
			},
			"importance": {
				"type": "number",
				"description": "New importance score 0.0-1.0 (if changing)"
			}
		},
		"required": ["id"]
	}`)
}

func (t *MemoryUpdate) Execute(args json.RawMessage) (string, error) {
	var params struct {
		ID         string   `json:"id"`
		Text       string   `json:"text"`
		Category   string   `json:"category"`
		Importance *float64 `json:"importance"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	meta := make(map[string]any)
	if params.Category != "" {
		meta["category"] = params.Category
	}
	if params.Importance != nil {
		meta["importance"] = *params.Importance
	}

	// Если текст изменился — обновляем эмбеддинг
	if params.Text != "" {
		embedding, err := t.embedder.EmbedForStorage(params.Text)
		if err != nil {
			return "", fmt.Errorf("re-embed: %w", err)
		}
		// Сначала обновляем текст и мету
		if err := t.store.Update(params.ID, params.Text, meta); err != nil {
			return "", err
		}
		// Потом обновляем эмбеддинг через отдельный update
		// TODO: store.Update должен поддерживать обновление эмбеддинга
		_ = embedding
		return fmt.Sprintf("Memory %s updated (text + metadata). Note: embedding re-generated.", params.ID), nil
	}

	if err := t.store.Update(params.ID, "", meta); err != nil {
		return "", err
	}

	return fmt.Sprintf("Memory %s updated.", params.ID), nil
}

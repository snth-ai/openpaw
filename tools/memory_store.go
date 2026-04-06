package tools

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/openpaw/server/memory"
	graphmem "github.com/openpaw/server/memory/graph"
	"github.com/openpaw/server/uid"
)

// MemoryStore — тул для сохранения воспоминаний.
// Роутинг: entity/fact/decision/preference → Graph + Memory Log. reflection → только Memory Log.
type MemoryStore struct {
	store      memory.Store
	embedder   *memory.Embedder
	graphStore graphmem.GraphStore // может быть nil
}

func NewMemoryStore(store memory.Store, embedder *memory.Embedder, graphStore graphmem.GraphStore) *MemoryStore {
	return &MemoryStore{store: store, embedder: embedder, graphStore: graphStore}
}

func (t *MemoryStore) Name() string { return "memory_store" }
func (t *MemoryStore) Description() string {
	return "Save a memory. Categories: entity (people/places/projects), fact (data/info), decision (agreements), preference (likes/dislikes) → saved to knowledge graph + memory log. reflection (your thoughts) → memory log only."
}

func (t *MemoryStore) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {
				"type": "string",
				"description": "The memory content to save"
			},
			"category": {
				"type": "string",
				"enum": ["preference", "fact", "decision", "entity", "reflection"],
				"description": "Category: entity/fact/decision/preference → knowledge graph. reflection → memory log only."
			},
			"importance": {
				"type": "number",
				"description": "How important (0.0-1.0). 1.0 = critical, 0.5 = normal, 0.1 = minor"
			},
			"min_strength": {
				"type": "number",
				"description": "Minimum strength floor — memory won't decay below this. 0.0 = casual (eats sushi), 0.3 = profession, 0.6 = life event (divorced), 0.8 = key relationship (wife), 0.9 = trauma (lost a child). Default 0."
			}
		},
		"required": ["text", "category", "importance"]
	}`)
}

func (t *MemoryStore) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Text        string  `json:"text"`
		Category    string  `json:"category"`
		Importance  float64 `json:"importance"`
		MinStrength float64 `json:"min_strength"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	// Эмбеддинг
	embedding, err := t.embedder.EmbedForStorage(params.Text)
	if err != nil {
		return "", fmt.Errorf("embed: %w", err)
	}

	// Всегда пишем в Memory Log
	m := &memory.Memory{
		Text:        params.Text,
		Category:    memory.Category(params.Category),
		ContentType: memory.ContentText,
		Importance:  params.Importance,
		Embedding:   embedding,
		CreatedAt:   time.Now(),
	}
	if err := t.store.Add(m); err != nil {
		return "", fmt.Errorf("store: %w", err)
	}

	result := fmt.Sprintf("Memory saved (id: %s, category: %s, importance: %.1f)", m.ID, m.Category, m.Importance)

	// Роутинг в граф: entity, fact, decision, preference → Graph
	if t.graphStore != nil && params.Category != "reflection" {
		go t.writeToGraph(params.Text, params.Category, params.Importance, params.MinStrength, embedding)
		result += fmt.Sprintf(" + graph (floor: %.1f)", params.MinStrength)
	}

	return result, nil
}

// writeToGraph создаёт ноду в графе из текста памяти.
func (t *MemoryStore) writeToGraph(text, category string, importance, minStrength float64, embedding []float32) {
	// Определяем тип ноды по category
	nodeType := "concept"
	switch category {
	case "entity":
		nodeType = "entity"
	case "preference":
		nodeType = "preference"
	case "fact":
		nodeType = "fact"
	case "decision":
		nodeType = "decision"
	}

	// Создаём ноду — текст как name (обрезаем до 100 символов)
	name := text
	if len(name) > 100 {
		name = name[:100]
	}

	node := &graphmem.Node{
		ID:           uid.New(),
		Name:         name,
		Type:         nodeType,
		Summary:      text,
		Aliases:      []string{},
		Tags:         []string{category},
		Properties:   map[string]any{"importance": importance, "min_strength": minStrength},
		Embedding:    embedding,
		MentionCount: 1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := t.graphStore.UpsertNode(node); err != nil {
		log.Printf("memory_store: graph write error: %v", err)
		return
	}

	log.Printf("memory_store: wrote to graph node %q (%s)", name, nodeType)
}

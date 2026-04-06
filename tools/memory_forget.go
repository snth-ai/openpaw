package tools

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/openpaw/server/memory"
	graphmem "github.com/openpaw/server/memory/graph"
)

// MemoryForget — тул для удаления воспоминаний из ОБОИХ слоёв (Memory Log + Graph).
type MemoryForget struct {
	store      memory.Store
	embedder   *memory.Embedder
	graphStore graphmem.GraphStore
}

func NewMemoryForget(store memory.Store, embedder *memory.Embedder, graphStore graphmem.GraphStore) *MemoryForget {
	return &MemoryForget{store: store, embedder: embedder, graphStore: graphStore}
}

func (t *MemoryForget) Name() string { return "memory_forget" }
func (t *MemoryForget) Description() string {
	return "Forget memories. Cleans both memory log and knowledge graph. Use query to forget by topic, or id for exact delete."
}

func (t *MemoryForget) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {
				"type": "string",
				"description": "ID of the memory to forget (exact delete from memory log)"
			},
			"query": {
				"type": "string",
				"description": "Forget everything related to this topic (fuzzy delete from both memory log and knowledge graph)"
			}
		}
	}`)
}

func (t *MemoryForget) Execute(args json.RawMessage) (string, error) {
	var params struct {
		ID    string `json:"id"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	if params.ID != "" {
		if err := t.store.Delete(params.ID); err != nil {
			return "", err
		}
		return fmt.Sprintf("Memory %s forgotten.", params.ID), nil
	}

	if params.Query != "" {
		var results []string

		// 1. Memory Log: забываем по embedding similarity
		embedding, err := t.embedder.EmbedForQuery(params.Query)
		if err != nil {
			return "", fmt.Errorf("embed: %w", err)
		}
		n, err := t.store.DeleteByQuery(embedding, 0.5, "")
		if err == nil && n > 0 {
			results = append(results, fmt.Sprintf("%d from memory log", n))
		}

		// 2. Graph: находим и инвалидируем связанные edges
		if t.graphStore != nil {
			graphForgotten := t.forgetFromGraph(params.Query, embedding)
			if graphForgotten > 0 {
				results = append(results, fmt.Sprintf("%d edges from knowledge graph", graphForgotten))
			}
		}

		if len(results) == 0 {
			return fmt.Sprintf("Nothing found to forget for %q.", params.Query), nil
		}

		return fmt.Sprintf("Forgotten: %s (query: %q).", joinStr(results, " + "), params.Query), nil
	}

	return "Provide either id or query to forget.", nil
}

// forgetFromGraph находит и инвалидирует edges по query.
func (t *MemoryForget) forgetFromGraph(query string, embedding []float32) int {
	forgotten := 0

	// По имени: ищем ноды, инвалидируем их edges
	nodes, _ := t.graphStore.FindNodesByNames([]string{query})

	// По embedding: ищем похожие ноды
	if len(nodes) == 0 {
		embNodes, _ := t.graphStore.FindNodesByEmbedding(embedding, 0.75, 5)
		nodes = embNodes
	}

	for _, node := range nodes {
		edges, _ := t.graphStore.GetActiveEdgesForNode(node.ID)
		for _, edge := range edges {
			err := t.graphStore.InvalidateEdge(edge.ID, "", "user requested forget: "+query)
			if err == nil {
				forgotten++
			}
		}
		log.Printf("memory_forget: invalidated %d edges for node %q", len(edges), node.Name)

		// Если нода осталась без active edges — удаляем целиком
		remaining, _ := t.graphStore.GetActiveEdgesForNode(node.ID)
		if len(remaining) == 0 {
			t.graphStore.DeleteNode(node.ID)
			log.Printf("memory_forget: deleted orphan node %q", node.Name)
		}
	}

	return forgotten
}

func joinStr(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

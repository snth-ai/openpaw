package graph

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/openpaw/server/llm"
	"github.com/openpaw/server/memory"
	"github.com/openpaw/server/uid"
)

// Resolver — 3-фазное разрешение сущностей.
type Resolver struct {
	store    GraphStore
	embedder *memory.Embedder
	provider llm.Provider // для LLM confirm merge (фаза 3)
}

func NewResolver(store GraphStore, embedder *memory.Embedder, provider llm.Provider) *Resolver {
	return &Resolver{store: store, embedder: embedder, provider: provider}
}

// Resolve находит или создаёт ноду для извлечённой сущности.
func (r *Resolver) Resolve(entity ExtractedEntity) (*Node, error) {
	// Phase 1: Exact match по name или aliases
	node, err := r.store.GetNodeByName(entity.Name)
	if err == nil && node != nil {
		r.mergeEntity(node, entity)
		return node, nil
	}

	for _, alias := range entity.Aliases {
		node, err = r.store.GetNodeByName(alias)
		if err == nil && node != nil {
			r.mergeEntity(node, entity)
			return node, nil
		}
	}

	// Phase 2: Embedding similarity
	if r.embedder != nil {
		embedding, err := r.embedder.EmbedForStorage(entity.Name + " " + entity.Summary)
		if err == nil {
			candidates, _ := r.store.FindNodesByEmbedding(embedding, 0.82, 3)
			if len(candidates) > 0 {
				// Phase 3: LLM confirmation
				if r.confirmMerge(entity, candidates[0]) {
					r.mergeEntity(candidates[0], entity)
					return candidates[0], nil
				}
			}
		}
	}

	// Не нашли — создаём новую ноду
	return r.createNode(entity)
}

func (r *Resolver) mergeEntity(node *Node, entity ExtractedEntity) {
	node.Aliases = mergeUnique(node.Aliases, entity.Aliases)
	node.Aliases = mergeUnique(node.Aliases, []string{entity.Name})
	// Убираем каноническое имя из aliases
	node.Aliases = removeStr(node.Aliases, node.Name)

	if entity.Summary != "" && len(entity.Summary) > len(node.Summary) {
		node.Summary = entity.Summary
	}
	if entity.Type != "" && entity.Type != "entity" {
		node.Type = entity.Type
	}
	node.UpdatedAt = time.Now()
	r.store.UpsertNode(node)
}

func (r *Resolver) createNode(entity ExtractedEntity) (*Node, error) {
	var embedding []float32
	if r.embedder != nil {
		emb, err := r.embedder.EmbedForStorage(entity.Name + " " + entity.Summary)
		if err == nil {
			embedding = emb
		}
	}

	nodeType := entity.Type
	if nodeType == "" {
		nodeType = "entity"
	}

	node := &Node{
		ID:           uid.New(),
		Name:         entity.Name,
		Type:         nodeType,
		Aliases:      entity.Aliases,
		Tags:         []string{},
		Summary:      entity.Summary,
		Properties:   map[string]any{},
		Embedding:    embedding,
		MentionCount: 1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := r.store.UpsertNode(node); err != nil {
		return nil, fmt.Errorf("create node: %w", err)
	}

	log.Printf("graph: created node %q (%s)", node.Name, node.Type)
	return node, nil
}

func (r *Resolver) confirmMerge(entity ExtractedEntity, candidate *Node) bool {
	if r.provider == nil {
		return false
	}

	prompt := fmt.Sprintf(`Определи, являются ли эти две сущности одним и тем же объектом.

Новая: "%s" (%s) — %s
Существующая: "%s" (%s) — %s
Aliases существующей: %s

Ответь ТОЛЬКО "yes" или "no".`,
		entity.Name, entity.Type, entity.Summary,
		candidate.Name, candidate.Type, candidate.Summary,
		strings.Join(candidate.Aliases, ", "))

	resp, err := r.provider.ChatStream([]llm.Message{
		{Role: "user", Content: prompt},
	}, nil, nil)
	if err != nil {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(resp.Content))
	return strings.HasPrefix(answer, "yes")
}

func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range a {
		lower := strings.ToLower(s)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		lower := strings.ToLower(s)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, s)
		}
	}
	return result
}

func removeStr(slice []string, val string) []string {
	var result []string
	for _, s := range slice {
		if !strings.EqualFold(s, val) {
			result = append(result, s)
		}
	}
	return result
}

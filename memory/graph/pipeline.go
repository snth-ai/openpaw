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

// Pipeline оркестрирует write path: extraction → resolve → store.
type Pipeline struct {
	store       GraphStore
	resolver    *Resolver
	invalidator *Invalidator
	provider    llm.Provider
	memoryStore memory.Store // для reflections → Memory Log
	embedder    *memory.Embedder
}

func NewPipeline(store GraphStore, provider llm.Provider, memStore memory.Store, embedder *memory.Embedder) *Pipeline {
	resolver := NewResolver(store, embedder, provider)
	invalidator := NewInvalidator(store, embedder)

	return &Pipeline{
		store:       store,
		resolver:    resolver,
		invalidator: invalidator,
		provider:    provider,
		memoryStore: memStore,
		embedder:    embedder,
	}
}

// ProcessDialogue извлекает граф из диалога и сохраняет.
func (p *Pipeline) ProcessDialogue(dialogue string, timestamp time.Time) error {
	// 1. LLM Extraction
	result, err := Extract(p.provider, dialogue)
	if err != nil {
		return err
	}

	log.Printf("graph: extracted %d entities, %d triplets, %d invalidations, %d reflections",
		len(result.Entities), len(result.Triplets), len(result.Invalidations), len(result.Reflections))

	// 2. Process invalidations FIRST
	for _, inv := range result.Invalidations {
		p.invalidator.Process(inv)
	}

	// 3. Resolve entities → nodes
	nodeMap := make(map[string]string) // entity name (lowercase) → node ID
	for _, entity := range result.Entities {
		node, err := p.resolver.Resolve(entity)
		if err != nil {
			log.Printf("graph: resolve entity %q error: %v", entity.Name, err)
			continue
		}
		nodeMap[strings.ToLower(entity.Name)] = node.ID
		for _, alias := range entity.Aliases {
			nodeMap[strings.ToLower(alias)] = node.ID
		}
	}

	// 4. Create edges from triplets
	var edgeIDs []string
	for _, triplet := range result.Triplets {
		fromID := nodeMap[strings.ToLower(triplet.Subject)]
		toID := nodeMap[strings.ToLower(triplet.Object)]

		if fromID == "" || toID == "" {
			// Попробуем resolve по имени напрямую
			if fromID == "" {
				fromNode, _ := p.store.GetNodeByName(triplet.Subject)
				if fromNode != nil {
					fromID = fromNode.ID
				}
			}
			if toID == "" {
				toNode, _ := p.store.GetNodeByName(triplet.Object)
				if toNode != nil {
					toID = toNode.ID
				}
			}
			if fromID == "" || toID == "" {
				continue
			}
		}

		// Проверяем дубликат
		existing, _ := p.store.FindDuplicateEdge(fromID, toID, triplet.Predicate)
		if existing != nil {
			existing.Strength = min(1.0, existing.Strength+0.05)
			p.store.UpdateEdge(existing)
			edgeIDs = append(edgeIDs, existing.ID)
			continue
		}

		edge := &Edge{
			ID:            uid.New(),
			FromID:        fromID,
			ToID:          toID,
			Relation:      triplet.Predicate,
			RelationGroup: triplet.RelationGroup,
			Strength:      triplet.Strength,
			MinStrength:   triplet.MinStrength,
			Context:       triplet.Context,
			CreatedAt:     timestamp,
			Properties:    map[string]any{},
		}

		if triplet.ValidFrom != "" {
			t, err := parseFlexibleDate(triplet.ValidFrom)
			if err == nil {
				edge.ValidFrom = &t
			}
		}

		if err := p.store.AddEdge(edge); err != nil {
			log.Printf("graph: add edge error: %v", err)
			continue
		}
		edgeIDs = append(edgeIDs, edge.ID)
	}

	// 5. Create episode
	nodeIDs := uniqueValues(nodeMap)
	episode := &Episode{
		ID:        uid.New(),
		Summary:   truncateStr(dialogue, 500),
		Timestamp: timestamp,
		NodeIDs:   nodeIDs,
		EdgeIDs:   edgeIDs,
	}
	p.store.AddEpisode(episode)

	// 6. Increment mention counts
	for _, nodeID := range nodeIDs {
		p.store.IncrementMentionCount(nodeID)
	}

	// 7. Send reflections to Memory Log
	if p.memoryStore != nil && p.embedder != nil {
		for _, ref := range result.Reflections {
			embedding, err := p.embedder.EmbedForStorage(ref.Text)
			if err != nil {
				continue
			}
			m := &memory.Memory{
				Text:        ref.Text,
				Category:    memory.Category(ref.Category),
				ContentType: memory.ContentText,
				Importance:  ref.Importance,
				Embedding:   embedding,
			}
			p.memoryStore.Add(m)
		}
	}

	log.Printf("graph: processed %d nodes, %d edges, 1 episode", len(nodeIDs), len(edgeIDs))
	return nil
}

func parseFlexibleDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"2006-01",
		"January 2006",
		"Jan 2006",
		time.RFC3339,
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date: %s", s)
}

func uniqueValues(m map[string]string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range m {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

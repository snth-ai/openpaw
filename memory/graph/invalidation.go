package graph

import (
	"log"
	"strings"

	"github.com/openpaw/server/memory"
)

// Invalidator обрабатывает инвалидацию фактов.
type Invalidator struct {
	store    GraphStore
	embedder *memory.Embedder
}

func NewInvalidator(store GraphStore, embedder *memory.Embedder) *Invalidator {
	return &Invalidator{store: store, embedder: embedder}
}

// Process обрабатывает одну инвалидацию.
// Ищет active edges чей context похож на old_fact и инвалидирует их.
func (inv *Invalidator) Process(extraction ExtractedInvalidation) {
	// Стратегия: ищем по ключевым словам в context всех active edges
	// (embedding search по контексту edges — TODO, пока text match)
	oldWords := tokenizeSimple(extraction.OldFact)
	if len(oldWords) == 0 {
		return
	}

	// Получаем все ноды, ищем их edges
	// Простой подход: ищем ноды чьё имя встречается в old_fact
	nodes, _ := inv.store.FindNodesByNames(oldWords)

	for _, node := range nodes {
		edges, _ := inv.store.GetActiveEdgesForNode(node.ID)
		for _, edge := range edges {
			if matchesContext(edge, extraction.OldFact) {
				inv.store.InvalidateEdge(edge.ID, "", extraction.Reason)
				log.Printf("graph: invalidated edge %s→%s→%s (%s)",
					edge.FromID, edge.Relation, edge.ToID, extraction.Reason)
			}
		}
	}
}

func matchesContext(edge Edge, fact string) bool {
	factLower := strings.ToLower(fact)
	// Проверяем совпадение по relation и context
	if edge.Context != "" && strings.Contains(factLower, strings.ToLower(edge.Context)) {
		return true
	}
	if strings.Contains(factLower, strings.ToLower(edge.Relation)) {
		return true
	}
	return false
}

func tokenizeSimple(text string) []string {
	text = strings.ToLower(text)
	words := strings.Fields(text)
	// Убираем короткие слова (предлоги и т.д.)
	var result []string
	for _, w := range words {
		if len(w) > 3 {
			result = append(result, w)
		}
	}
	return result
}

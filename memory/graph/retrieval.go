package graph

import (
	"strings"
	"unicode"

	"github.com/openpaw/server/memory"
)

// Retriever — read path: entity detection + subgraph retrieval.
type Retriever struct {
	store    GraphStore
	embedder *memory.Embedder
}

func NewRetriever(store GraphStore, embedder *memory.Embedder) *Retriever {
	return &Retriever{store: store, embedder: embedder}
}

// RetrieveContext находит релевантные знания для инжекта в промпт.
// Вызывается на каждое сообщение (лёгкий, без API calls).
func (r *Retriever) RetrieveContext(userMessage string) string {
	// 1. Entity detection — находим упоминания нод в тексте
	detectedNodes := r.detectEntities(userMessage)
	if len(detectedNodes) == 0 {
		return ""
	}

	// 2. Собираем node IDs
	var nodeIDs []string
	for _, n := range detectedNodes {
		nodeIDs = append(nodeIDs, n.ID)
	}

	// 3. Получаем subgraph (active edges only)
	sg, err := r.store.GetSubgraph(nodeIDs, false)
	if err != nil || len(sg.Edges) == 0 {
		return ""
	}

	// 4. Render
	return RenderSubgraph(sg)
}

// RetrieveByEmbedding — поиск по embedding (для memory_recall tool).
func (r *Retriever) RetrieveByEmbedding(query string, limit int) string {
	if r.embedder == nil {
		return ""
	}

	embedding, err := r.embedder.EmbedForQuery(query)
	if err != nil {
		return ""
	}

	nodes, err := r.store.FindNodesByEmbedding(embedding, 0.75, limit)
	if err != nil || len(nodes) == 0 {
		return ""
	}

	var nodeIDs []string
	for _, n := range nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}

	sg, err := r.store.GetSubgraph(nodeIDs, false)
	if err != nil {
		return ""
	}

	return RenderSubgraph(sg)
}

// detectEntities — находит упоминания нод графа в тексте.
// Лёгкий, без API calls. SQL запрос по именам и aliases.
func (r *Retriever) detectEntities(text string) []*Node {
	// Токенизируем текст — слова и n-граммы (2-3 слова)
	tokens := tokenizeText(text)
	ngrams := generateNgrams(tokens, 3)
	allCandidates := append(tokens, ngrams...)

	if len(allCandidates) == 0 {
		return nil
	}

	nodes, err := r.store.FindNodesByNames(allCandidates)
	if err != nil {
		return nil
	}
	return nodes
}

// tokenizeText разбивает на слова, убирает пунктуацию.
func tokenizeText(text string) []string {
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// generateNgrams генерирует n-граммы до maxN слов.
func generateNgrams(tokens []string, maxN int) []string {
	var ngrams []string
	for n := 2; n <= maxN && n <= len(tokens); n++ {
		for i := 0; i <= len(tokens)-n; i++ {
			ngram := strings.Join(tokens[i:i+n], " ")
			ngrams = append(ngrams, ngram)
		}
	}
	return ngrams
}

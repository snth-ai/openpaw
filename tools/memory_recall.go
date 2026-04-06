package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openpaw/server/memory"
)

// MemoryRecall — тул для поиска по памяти.
type MemoryRecall struct {
	store    memory.Store
	embedder *memory.Embedder
	reranker *memory.Reranker
}

func NewMemoryRecall(store memory.Store, embedder *memory.Embedder, reranker *memory.Reranker) *MemoryRecall {
	return &MemoryRecall{store: store, embedder: embedder, reranker: reranker}
}

func (t *MemoryRecall) Name() string { return "memory_recall" }
func (t *MemoryRecall) Description() string {
	return "Search your memories for relevant information. Returns the most relevant memories sorted by relevance."
}

func (t *MemoryRecall) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "What to search for in memory"
			},
			"limit": {
				"type": "integer",
				"description": "Max results to return (default 5)"
			}
		},
		"required": ["query"]
	}`)
}

func (t *MemoryRecall) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}
	if params.Limit <= 0 {
		params.Limit = 5
	}

	// Vector search
	embedding, err := t.embedder.EmbedForQuery(params.Query)
	if err != nil {
		return "", fmt.Errorf("embed query: %w", err)
	}

	vectorCandidates, err := t.store.Search(embedding, params.Limit*4, "")
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	// BM25 keyword search
	allMems, _ := t.store.All("")
	bm25Results := memory.RankBM25(params.Query, allMems, params.Limit*4)

	// Merge + deduplicate (vector 70% weight, BM25 30%)
	candidates := mergeResults(vectorCandidates, bm25Results)

	if len(candidates) == 0 {
		return "No memories found.", nil
	}

	// Реранкинг через Gemini Flash
	results := candidates
	if t.reranker != nil && len(candidates) > params.Limit {
		reranked, err := t.reranker.Rerank(params.Query, candidates, params.Limit)
		if err == nil {
			results = reranked
		}
	}

	if len(results) > params.Limit {
		results = results[:params.Limit]
	}

	// Boost access для найденных записей
	cfg := memory.DefaultDecayConfig()
	for i := range results {
		memory.BoostAccess(&results[i].Memory, cfg)
	}

	// Форматируем результат
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s] (importance: %.1f) %s\n",
			i+1, r.Category, r.Importance, r.Text))
	}

	return sb.String(), nil
}

// AutoRecall делает автоматический recall для инжекта в system prompt.
// Вызывается перед каждым ответом LLM.
func (t *MemoryRecall) AutoRecall(userMessage string, limit int) string {
	if limit <= 0 {
		limit = 5
	}

	embedding, err := t.embedder.EmbedForQuery(userMessage)
	if err != nil {
		return ""
	}

	vectorCandidates, err := t.store.Search(embedding, limit*4, "")
	if err != nil {
		vectorCandidates = nil
	}

	allMems, _ := t.store.All("")
	bm25Results := memory.RankBM25(userMessage, allMems, limit*4)

	candidates := mergeResults(vectorCandidates, bm25Results)
	if len(candidates) == 0 {
		return ""
	}

	results := candidates
	if t.reranker != nil && len(candidates) > limit {
		reranked, err := t.reranker.Rerank(userMessage, candidates, limit)
		if err == nil {
			results = reranked
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}

	// Фильтруем по минимальному score (если реранкер отработал)
	var relevant []memory.SearchResult
	for _, r := range results {
		if r.RerankerScore >= 0.3 || r.Distance < 0.5 {
			relevant = append(relevant, r)
		}
	}

	if len(relevant) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<relevant-memories>\n")
	for _, r := range relevant {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", r.Category, r.Text))
	}
	sb.WriteString("</relevant-memories>")

	return sb.String()
}

// mergeResults объединяет vector и BM25 результаты, дедуплицирует.
func mergeResults(vector, bm25 []memory.SearchResult) []memory.SearchResult {
	seen := make(map[string]bool)
	var merged []memory.SearchResult

	// Vector results first (primary)
	for _, r := range vector {
		if !seen[r.ID] {
			seen[r.ID] = true
			merged = append(merged, r)
		}
	}

	// BM25 results add diversity
	for _, r := range bm25 {
		if !seen[r.ID] {
			seen[r.ID] = true
			merged = append(merged, r)
		}
	}

	return merged
}

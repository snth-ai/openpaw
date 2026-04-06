package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

const geminiFlashURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-lite-preview:generateContent"

// Reranker переранжирует кандидатов через Gemini Flash.
type Reranker struct {
	apiKey string
	client *http.Client
}

func NewReranker(apiKey string) *Reranker {
	return &Reranker{apiKey: apiKey, client: &http.Client{}}
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig map[string]any         `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Rerank переранжирует кандидатов по релевантности к query.
// Возвращает отсортированные результаты с reranker_score.
func (r *Reranker) Rerank(query string, candidates []SearchResult, topN int) ([]SearchResult, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) <= topN {
		topN = len(candidates)
	}

	// Формируем промпт для Gemini Flash
	var sb strings.Builder
	sb.WriteString("You are a relevance scoring system. Given a query and a list of text candidates, ")
	sb.WriteString("rate each candidate's relevance to the query on a scale of 0.0 to 1.0.\n")
	sb.WriteString("Respond ONLY with a JSON array of numbers (scores), one per candidate, in the same order.\n\n")
	sb.WriteString(fmt.Sprintf("Query: %s\n\nCandidates:\n", query))

	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c.Text))
	}

	body := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: sb.String()}}},
		},
		GenerationConfig: map[string]any{
			"temperature":     0.0,
			"maxOutputTokens": 256,
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	req, err := http.NewRequest("POST", geminiFlashURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank request: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response: %w", err)
	}

	var result geminiResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("unmarshal rerank response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("gemini rerank error: %s", result.Error.Message)
	}

	// Парсим скоры из ответа
	scores := parseScores(result, len(candidates))

	for i := range candidates {
		if i < len(scores) {
			candidates[i].RerankerScore = scores[i]
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].RerankerScore > candidates[j].RerankerScore
	})

	if topN < len(candidates) {
		candidates = candidates[:topN]
	}

	return candidates, nil
}

// parseScores извлекает массив float из ответа Gemini.
func parseScores(resp geminiResponse, expected int) []float64 {
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return make([]float64, expected)
	}

	text := resp.Candidates[0].Content.Parts[0].Text
	text = strings.TrimSpace(text)

	// Убираем markdown code fence если есть
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var scores []float64
	if err := json.Unmarshal([]byte(text), &scores); err != nil {
		// Фоллбэк: все скоры = 0.5
		scores = make([]float64, expected)
		for i := range scores {
			scores[i] = 0.5
		}
	}

	return scores
}

package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	geminiEmbedURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-2-preview:embedContent"
	EmbeddingDims  = 768
)

// Embedder генерирует эмбеддинги через Gemini Embedding 2.
type Embedder struct {
	apiKey string
	client *http.Client
}

func NewEmbedder(apiKey string) *Embedder {
	return &Embedder{apiKey: apiKey, client: &http.Client{}}
}

type embedRequest struct {
	Content            embedContent `json:"content"`
	TaskType           string       `json:"taskType,omitempty"`
	OutputDimensionality int        `json:"outputDimensionality"`
}

type embedContent struct {
	Parts []embedPart `json:"parts"`
}

type embedPart struct {
	Text string `json:"text,omitempty"`
}

type embedResponse struct {
	Embedding struct {
		Values []float32 `json:"values"`
	} `json:"embedding"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// EmbedForStorage генерирует эмбеддинг для сохранения (RETRIEVAL_DOCUMENT).
func (e *Embedder) EmbedForStorage(text string) ([]float32, error) {
	return e.embed(text, "RETRIEVAL_DOCUMENT")
}

// EmbedForQuery генерирует эмбеддинг для поиска (RETRIEVAL_QUERY).
func (e *Embedder) EmbedForQuery(text string) ([]float32, error) {
	return e.embed(text, "RETRIEVAL_QUERY")
}

func (e *Embedder) embed(text, taskType string) ([]float32, error) {
	body := embedRequest{
		Content:              embedContent{Parts: []embedPart{{Text: text}}},
		TaskType:             taskType,
		OutputDimensionality: EmbeddingDims,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequest("POST", geminiEmbedURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	var result embedResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("unmarshal embed response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("gemini embed error: %s", result.Error.Message)
	}

	if len(result.Embedding.Values) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}

	return result.Embedding.Values, nil
}

package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// WebSearch — тул для поиска в интернете через Perplexity Sonar.
type WebSearch struct{}

func (t WebSearch) Name() string { return "web_search" }
func (t WebSearch) Description() string {
	return "Search the web for current information. Returns relevant results with snippets. Use when you need up-to-date facts, news, or information you don't have."
}

func (t WebSearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Search query"
			}
		},
		"required": ["query"]
	}`)
}

func (t WebSearch) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	apiKey := os.Getenv("PERPLEXITY_API_KEY")
	if apiKey != "" {
		return searchPerplexity(params.Query, apiKey)
	}

	googleKey := os.Getenv("GOOGLE_SEARCH_API_KEY")
	googleCX := os.Getenv("GOOGLE_SEARCH_CX")
	if googleKey != "" && googleCX != "" {
		return searchGoogle(params.Query, googleKey, googleCX)
	}

	return "", fmt.Errorf("no search API configured (set PERPLEXITY_API_KEY or GOOGLE_SEARCH_API_KEY + GOOGLE_SEARCH_CX)")
}

func searchPerplexity(query, apiKey string) (string, error) {
	body := map[string]any{
		"model": "sonar",
		"messages": []map[string]string{
			{"role": "user", "content": query},
		},
	}

	payload, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", "https://api.perplexity.ai/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("perplexity request: %w", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("perplexity %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Citations []string `json:"citations"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("parse perplexity response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "No results found.", nil
	}

	var sb strings.Builder
	sb.WriteString(result.Choices[0].Message.Content)

	if len(result.Citations) > 0 {
		sb.WriteString("\n\nSources:\n")
		for i, url := range result.Citations {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, url))
		}
	}

	return sb.String(), nil
}

func searchGoogle(query, apiKey, cx string) (string, error) {
	url := fmt.Sprintf("https://www.googleapis.com/customsearch/v1?key=%s&cx=%s&q=%s&num=5",
		apiKey, cx, query)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("google search: %w", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("google %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Items []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return "", fmt.Errorf("parse google response: %w", err)
	}

	if len(result.Items) == 0 {
		return "No results found.", nil
	}

	var sb strings.Builder
	for i, item := range result.Items {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n\n", i+1, item.Title, item.Snippet, item.Link))
	}

	return sb.String(), nil
}

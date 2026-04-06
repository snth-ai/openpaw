package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ProviderType identifies the provider for provider-specific behavior.
type ProviderType string

const (
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderXAI        ProviderType = "xai"
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderOpenAI     ProviderType = "openai"
	ProviderGeneric    ProviderType = "generic"
)

// OpenAICompat implements Provider via any OpenAI-compatible chat completions API.
// Works with OpenRouter, xAI, Anthropic, OpenAI, and any compatible endpoint.
type OpenAICompat struct {
	APIKey       string
	Model        string
	BaseURL      string       // full URL to chat completions endpoint
	ProviderKind ProviderType // for provider-specific behavior
	ServiceName  string       // for logging/tracking ("openrouter", "xai", etc.)
}

// NewOpenRouter creates a provider configured for OpenRouter.
func NewOpenRouter(apiKey, model string) *OpenAICompat {
	return &OpenAICompat{
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://openrouter.ai/api/v1/chat/completions",
		ProviderKind: ProviderOpenRouter,
		ServiceName:  "openrouter",
	}
}

// NewXAI creates a provider configured for xAI direct API.
func NewXAI(apiKey, model string) *OpenAICompat {
	return &OpenAICompat{
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://api.x.ai/v1/chat/completions",
		ProviderKind: ProviderXAI,
		ServiceName:  "xai",
	}
}

// NewAnthropic creates a provider configured for Anthropic direct API (Messages API, OpenAI-compat layer).
func NewAnthropic(apiKey, model string) *OpenAICompat {
	return &OpenAICompat{
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      "https://api.anthropic.com/v1/messages",
		ProviderKind: ProviderAnthropic,
		ServiceName:  "anthropic",
	}
}

// NewGenericOpenAI creates a provider for any OpenAI-compatible endpoint.
func NewGenericOpenAI(apiKey, model, baseURL, serviceName string) *OpenAICompat {
	return &OpenAICompat{
		APIKey:       apiKey,
		Model:        model,
		BaseURL:      baseURL,
		ProviderKind: ProviderGeneric,
		ServiceName:  serviceName,
	}
}

type request struct {
	Model         string            `json:"model"`
	Messages      []Message         `json:"messages"`
	Tools         []map[string]any  `json:"tools,omitempty"`
	Stream        bool              `json:"stream"`
	StreamOptions *streamOptions    `json:"stream_options,omitempty"`
	Provider      *providerPrefs    `json:"provider,omitempty"`
	Store         *bool             `json:"store,omitempty"` // xAI: don't store on their servers
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type providerPrefs struct {
	Order  []string `json:"order,omitempty"`
	Ignore []string `json:"ignore,omitempty"`
}

func (o *OpenAICompat) ChatStream(messages []Message, tools []map[string]any, onChunk func(string)) (*Response, error) {
	// Prepare messages based on provider
	msgs := o.prepareMessages(messages)

	body := request{
		Model:         o.Model,
		Messages:      msgs,
		Tools:         tools,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}

	// Provider-specific request tweaks
	switch o.ProviderKind {
	case ProviderOpenRouter:
		// Pin to Anthropic for Claude models (cache works only on Anthropic, not Vertex/Bedrock)
		if strings.HasPrefix(o.Model, "anthropic/") {
			body.Provider = &providerPrefs{
				Order:  []string{"Anthropic"},
				Ignore: []string{"Google", "Amazon Bedrock"},
			}
		}
	case ProviderXAI:
		// Don't store conversations on xAI servers
		f := false
		body.Store = &f
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", o.BaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s %d: %s", o.ServiceName, resp.StatusCode, string(b))
	}

	return parseSSE(resp.Body, onChunk)
}

// prepareMessages adjusts messages for provider compatibility.
func (o *OpenAICompat) prepareMessages(messages []Message) []Message {
	switch o.ProviderKind {
	case ProviderOpenRouter:
		// OpenRouter/Anthropic supports cache_control in ContentParts — send as-is
		return messages

	default:
		// Other providers: strip ContentParts, convert to plain string content
		// cache_control is Anthropic-specific and would confuse other APIs
		result := make([]Message, len(messages))
		for i, m := range messages {
			if len(m.ContentParts) > 0 {
				// Flatten ContentParts to plain string
				var sb strings.Builder
				for _, p := range m.ContentParts {
					sb.WriteString(p.Text)
				}
				result[i] = Message{
					Role:       m.Role,
					Content:    sb.String(),
					ToolCalls:  m.ToolCalls,
					ToolCallID: m.ToolCallID,
				}
			} else {
				result[i] = m
			}
		}
		return result
	}
}

// parseSSE parses Server-Sent Events from any OpenAI-compatible streaming endpoint.
func parseSSE(r io.Reader, onChunk func(string)) (*Response, error) {
	scanner := bufio.NewScanner(r)
	// Increase scanner buffer for large SSE chunks
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var content strings.Builder
	var toolCalls []ToolCall
	var usageInfo Usage
	toolArgs := make(map[int]*strings.Builder)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int          `json:"index"`
						ID       string       `json:"id"`
						Type     string       `json:"type"`
						Function FunctionCall `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *Usage `json:"usage,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		// Usage arrives in final chunks
		if chunk.Usage != nil {
			usageInfo = *chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			content.WriteString(delta.Content)
			if onChunk != nil {
				onChunk(delta.Content)
			}
		}

		for _, tc := range delta.ToolCalls {
			for len(toolCalls) <= tc.Index {
				toolCalls = append(toolCalls, ToolCall{Type: "function"})
			}
			if tc.ID != "" {
				toolCalls[tc.Index].ID = tc.ID
			}
			if tc.Function.Name != "" {
				toolCalls[tc.Index].Function.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				if _, ok := toolArgs[tc.Index]; !ok {
					toolArgs[tc.Index] = &strings.Builder{}
				}
				toolArgs[tc.Index].WriteString(tc.Function.Arguments)
			}
		}
	}

	for i, sb := range toolArgs {
		if i < len(toolCalls) {
			toolCalls[i].Function.Arguments = sb.String()
		}
	}

	return &Response{
		Content:   content.String(),
		ToolCalls: toolCalls,
		Usage:     usageInfo,
	}, scanner.Err()
}

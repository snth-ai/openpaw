package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const xaiResponsesURL = "https://api.x.ai/v1/responses"

// ImageAnalyzer analyzes images via xAI Grok vision API.
type ImageAnalyzer struct {
	apiKey string
	model  string
	client *http.Client
}

func NewImageAnalyzer(apiKey string) *ImageAnalyzer {
	return &ImageAnalyzer{
		apiKey: apiKey,
		model:  "grok-4-1-fast-non-reasoning",
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Analyze sends an image to Grok and returns a text description.
// imageData can be raw bytes or base64 string.
// mimeType should be "image/jpeg" or "image/png".
// prompt is the user's question about the image (can be empty for default description).
func (a *ImageAnalyzer) Analyze(imageData []byte, mimeType, prompt string) (string, error) {
	if prompt == "" {
		prompt = "Describe this image in detail. Include: people (appearance, hair, body), their actions, expressions, setting, lighting, mood. Be factual and descriptive. Answer in Russian."
	}

	b64 := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)

	reqBody := fmt.Sprintf(`{
		"model": "%s",
		"input": [{
			"role": "user",
			"content": [
				{"type": "input_image", "image_url": "%s", "detail": "high"},
				{"type": "input_text", "text": %s}
			]
		}],
		"temperature": 0.3
	}`, a.model, dataURL, mustJSONString(prompt))

	req, err := http.NewRequest("POST", xaiResponsesURL, strings.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("xai request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var result xaiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("xai error: %s", *result.Error)
	}

	for _, item := range result.Output {
		if item.Type == "message" {
			for _, c := range item.Content {
				if c.Type == "output_text" && c.Text != "" {
					return c.Text, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no text in response (status: %s)", result.Status)
}

// AnalyzeFile reads a file from disk and analyzes it.
func (a *ImageAnalyzer) AnalyzeFile(path, prompt string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	mime := "image/jpeg"
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".png") {
		mime = "image/png"
	} else if strings.HasSuffix(lower, ".webp") {
		mime = "image/webp"
	}

	return a.Analyze(data, mime, prompt)
}

// AnalyzeURL downloads an image from URL and analyzes it.
func (a *ImageAnalyzer) AnalyzeURL(imageURL, prompt string) (string, error) {
	resp, err := a.client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}

	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "image/jpeg"
	}

	return a.Analyze(data, mime, prompt)
}

type xaiResponse struct {
	Output []xaiOutputItem `json:"output"`
	Status string          `json:"status"`
	Error  *string         `json:"error"`
}

type xaiOutputItem struct {
	Type    string           `json:"type"`
	Content []xaiContentItem `json:"content"`
}

type xaiContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func mustJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// --- Tool interface implementation ---

// ImageTool wraps ImageAnalyzer as a Tool for the registry.
type ImageTool struct {
	analyzer *ImageAnalyzer
}

func NewImageTool(xaiAPIKey string) *ImageTool {
	return &ImageTool{analyzer: NewImageAnalyzer(xaiAPIKey)}
}

func (t *ImageTool) Name() string { return "image" }
func (t *ImageTool) Description() string {
	return "Analyze an image from a file path or URL. Returns a detailed text description of the image contents."
}

func (t *ImageTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Local file path to the image"
			},
			"url": {
				"type": "string",
				"description": "URL of the image to analyze"
			},
			"prompt": {
				"type": "string",
				"description": "Question or instruction about the image (optional, defaults to general description)"
			}
		}
	}`)
}

func (t *ImageTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Path   string `json:"path"`
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	if params.Path != "" {
		return t.analyzer.AnalyzeFile(params.Path, params.Prompt)
	}
	if params.URL != "" {
		return t.analyzer.AnalyzeURL(params.URL, params.Prompt)
	}

	return "", fmt.Errorf("either 'path' or 'url' is required")
}

// GetAnalyzer returns the underlying ImageAnalyzer for direct use (e.g., from Telegram handler).
func (t *ImageTool) GetAnalyzer() *ImageAnalyzer {
	return t.analyzer
}

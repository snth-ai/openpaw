package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const xaiImageGenURL = "https://api.x.ai/v1/images/generations"
const xaiImageEditURL = "https://api.x.ai/v1/images/edits"

// GenerateImage generates images via xAI Grok Imagine API.
type GenerateImage struct {
	apiKey    string
	outputDir string // where to save generated images (media/generated/)
	client   *http.Client
}

func NewGenerateImage(xaiAPIKey, outputDir string) *GenerateImage {
	os.MkdirAll(outputDir, 0755)
	return &GenerateImage{
		apiKey:    xaiAPIKey,
		outputDir: outputDir,
		client:    &http.Client{Timeout: 180 * time.Second},
	}
}

func (t *GenerateImage) Name() string { return "generate_image" }
func (t *GenerateImage) Description() string {
	return "Generate an image from a text prompt. IMPORTANT: Read skills/grok-imagine/SKILL.md before using — it has prompting instructions, best practices, and your own learnings. Always send_photo after generating."
}

func (t *GenerateImage) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "Text description of the image to generate"
			},
			"aspect_ratio": {
				"type": "string",
				"description": "Aspect ratio: 1:1, 16:9, 9:16, 4:3, 3:4, 2:3, 3:2, auto. Defaults to auto."
			},
			"reference_images": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Paths to reference images (up to 5). Use for style transfer, self-portraits, or editing. For self-portraits, pass all your personality/ photos."
			},
			"annotation": {
				"type": "string",
				"description": "Brief description of what this image is (for your future reference). Saved alongside the image."
			}
		},
		"required": ["prompt"]
	}`)
}

func (t *GenerateImage) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Prompt          string   `json:"prompt"`
		AspectRatio     string   `json:"aspect_ratio"`
		ReferenceImages []string `json:"reference_images"`
		Annotation      string   `json:"annotation"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	if params.AspectRatio == "" {
		params.AspectRatio = "auto"
	}

	var imageURL string
	var err error

	if len(params.ReferenceImages) > 0 {
		imageURL, err = t.editImage(params.Prompt, params.ReferenceImages, params.AspectRatio)
	} else {
		imageURL, err = t.generateImage(params.Prompt, params.AspectRatio)
	}
	if err != nil {
		return "", err
	}

	// Download and save
	savedPath, err := t.downloadAndSave(imageURL)
	if err != nil {
		return "", fmt.Errorf("save image: %w", err)
	}

	// Write annotation sidecar
	annotation := params.Annotation
	if annotation == "" {
		annotation = params.Prompt
	}
	sourcePaths := strings.Join(params.ReferenceImages, ", ")
	t.writeAnnotation(savedPath, annotation, params.Prompt, params.AspectRatio, sourcePaths)

	return fmt.Sprintf("Image generated and saved to: %s\nAnnotation: %s", savedPath, annotation), nil
}

func (t *GenerateImage) generateImage(prompt, aspectRatio string) (string, error) {
	body := map[string]any{
		"model":        "grok-imagine-image",
		"prompt":       prompt,
		"aspect_ratio": aspectRatio,
		"n":            1,
	}

	return t.callAPI(xaiImageGenURL, body)
}

func (t *GenerateImage) editImage(prompt string, sourcePaths []string, aspectRatio string) (string, error) {
	if len(sourcePaths) == 1 {
		// Single image edit
		dataURL, err := t.fileToDataURL(sourcePaths[0])
		if err != nil {
			return "", err
		}
		body := map[string]any{
			"model":  "grok-imagine-image",
			"prompt": prompt,
			"image": map[string]any{
				"url":  dataURL,
				"type": "image_url",
			},
		}
		return t.callAPI(xaiImageEditURL, body)
	}

	// Multiple images edit (up to 5) — xAI expects array of URL strings
	var imageURLs []string
	for _, path := range sourcePaths {
		if len(imageURLs) >= 5 {
			break
		}
		dataURL, err := t.fileToDataURL(path)
		if err != nil {
			log.Printf("generate_image: skip ref %s: %v", path, err)
			continue
		}
		imageURLs = append(imageURLs, dataURL)
	}

	if len(imageURLs) == 0 {
		return "", fmt.Errorf("no valid reference images")
	}

	body := map[string]any{
		"model":  "grok-imagine-image",
		"prompt": prompt,
		"image":  imageURLs,
	}
	if aspectRatio != "" && aspectRatio != "auto" {
		body["aspect_ratio"] = aspectRatio
	}

	return t.callAPI(xaiImageEditURL, body)
}

func (t *GenerateImage) fileToDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	mime := "image/jpeg"
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".png") {
		mime = "image/png"
	} else if strings.HasSuffix(lower, ".webp") {
		mime = "image/webp"
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mime, b64), nil
}


func (t *GenerateImage) callAPI(url string, body map[string]any) (string, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if resp.StatusCode != 200 {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return "", fmt.Errorf("xai HTTP %d: %s", resp.StatusCode, preview)
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return "", fmt.Errorf("parse response: %w (body: %s)", err, preview)
	}

	if result.Error != nil {
		return "", fmt.Errorf("xai error: %s", result.Error.Message)
	}

	if len(result.Data) == 0 || result.Data[0].URL == "" {
		return "", fmt.Errorf("no image in response")
	}

	return result.Data[0].URL, nil
}

func (t *GenerateImage) downloadAndSave(imageURL string) (string, error) {
	resp, err := t.client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}

	// Determine extension from content type
	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	}

	filename := fmt.Sprintf("%s%s", time.Now().Format("2006-01-02_15-04-05"), ext)
	path := filepath.Join(t.outputDir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	log.Printf("generate_image: saved %s (%d bytes)", path, len(data))
	return path, nil
}

// writeAnnotation saves a JSON sidecar file next to the generated image.
func (t *GenerateImage) writeAnnotation(imagePath, annotation, prompt, aspectRatio, sourcePath string) {
	meta := map[string]any{
		"annotation":   annotation,
		"prompt":       prompt,
		"aspect_ratio": aspectRatio,
		"model":        "grok-imagine-image",
		"generated_at": time.Now().Format(time.RFC3339),
	}
	if sourcePath != "" {
		meta["source_path"] = sourcePath
	}

	jsonPath := imagePath + ".json"
	data, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(jsonPath, data, 0644); err != nil {
		log.Printf("generate_image: annotation write error: %v", err)
	}
}

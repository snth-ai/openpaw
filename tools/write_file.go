package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile — тул для записи/создания файлов.
type WriteFile struct{}

func (t WriteFile) Name() string { return "write_file" }
func (t WriteFile) Description() string {
	return "Write content to a file. Creates the file and parent directories if they don't exist. Overwrites existing content."
}

func (t WriteFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the file to write"
			},
			"content": {
				"type": "string",
				"description": "Content to write to the file"
			}
		},
		"required": ["path", "content"]
	}`)
}

func (t WriteFile) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	// Создаём директории если нужно
	if err := os.MkdirAll(filepath.Dir(params.Path), 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(params.Path, []byte(params.Content), 0644); err != nil {
		return "", fmt.Errorf("write %s: %w", params.Path, err)
	}

	return fmt.Sprintf("Written %d bytes to %s", len(params.Content), params.Path), nil
}

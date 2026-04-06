package tools

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReadFile — тул для чтения файлов.
type ReadFile struct{}

func (t ReadFile) Name() string { return "read_file" }
func (t ReadFile) Description() string {
	return "Read the contents of a file. Returns the file content as text."
}

func (t ReadFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the file to read"
			}
		},
		"required": ["path"]
	}`)
}

func (t ReadFile) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", params.Path, err)
	}

	// Лимит чтобы не забить контекст
	content := string(data)
	if len(content) > 50000 {
		content = content[:50000] + "\n... (truncated)"
	}

	return content, nil
}

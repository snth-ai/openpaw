package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Find — find files by glob pattern.
type Find struct{}

func (t Find) Name() string { return "find" }
func (t Find) Description() string {
	return "Find files matching a glob pattern. Supports ** for recursive search. Returns matching file paths."
}

func (t Find) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern (e.g. \"*.md\", \"**/*.go\", \"docs/**\")"
			},
			"path": {
				"type": "string",
				"description": "Root directory to search from (defaults to current dir)"
			}
		},
		"required": ["pattern"]
	}`)
}

const findMaxResults = 500

func (t Find) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	root := params.Path
	if root == "" {
		root = "."
	}

	// If pattern contains **, do recursive walk
	if strings.Contains(params.Pattern, "**") {
		return findRecursive(root, params.Pattern)
	}

	// Simple glob
	fullPattern := filepath.Join(root, params.Pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return "", fmt.Errorf("glob %s: %w", fullPattern, err)
	}

	if len(matches) == 0 {
		return "(no matches)", nil
	}

	if len(matches) > findMaxResults {
		matches = matches[:findMaxResults]
	}

	return strings.Join(matches, "\n"), nil
}

func findRecursive(root, pattern string) (string, error) {
	// Extract the file pattern after **/ (e.g., "**/*.go" → "*.go")
	parts := strings.SplitN(pattern, "**/", 2)
	filePattern := "*"
	if len(parts) > 1 && parts[1] != "" {
		filePattern = parts[1]
	}

	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			// Skip hidden dirs
			if strings.HasPrefix(info.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		matched, _ := filepath.Match(filePattern, info.Name())
		if matched {
			matches = append(matches, path)
			if len(matches) >= findMaxResults {
				return fmt.Errorf("limit reached")
			}
		}
		return nil
	})

	if err != nil && len(matches) == 0 {
		return "", err
	}

	if len(matches) == 0 {
		return "(no matches)", nil
	}

	result := strings.Join(matches, "\n")
	if len(matches) >= findMaxResults {
		result += fmt.Sprintf("\n... (showing first %d results)", findMaxResults)
	}
	return result, nil
}

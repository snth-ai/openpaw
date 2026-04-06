package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Grep — search file contents for a pattern.
type Grep struct{}

func (t Grep) Name() string { return "grep" }
func (t Grep) Description() string {
	return "Search file contents for a text pattern. Can search a single file or recursively in a directory. Returns matching lines with file paths and line numbers."
}

func (t Grep) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Text pattern to search for (case-insensitive substring match)"
			},
			"path": {
				"type": "string",
				"description": "File or directory to search in (defaults to current dir)"
			},
			"glob": {
				"type": "string",
				"description": "File glob filter when searching directories (e.g. \"*.go\", \"*.md\"). Defaults to all files."
			}
		},
		"required": ["pattern"]
	}`)
}

const (
	grepMaxMatches    = 200
	grepMaxLineLen    = 500
	grepMaxResultSize = 30000
)

func (t Grep) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	if params.Pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

	path := params.Path
	if path == "" {
		path = "."
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}

	var matches []string
	needle := strings.ToLower(params.Pattern)

	if info.IsDir() {
		filepath.Walk(path, func(fpath string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				if strings.HasPrefix(fi.Name(), ".") && fpath != path {
					return filepath.SkipDir
				}
				return nil
			}
			// Apply glob filter
			if params.Glob != "" {
				matched, _ := filepath.Match(params.Glob, fi.Name())
				if !matched {
					return nil
				}
			}
			// Skip binary files (simple heuristic: skip if > 1MB or has null bytes)
			if fi.Size() > 1<<20 {
				return nil
			}
			fileMatches := grepFile(fpath, needle)
			matches = append(matches, fileMatches...)
			if len(matches) >= grepMaxMatches {
				return fmt.Errorf("limit")
			}
			return nil
		})
	} else {
		matches = grepFile(path, needle)
	}

	if len(matches) == 0 {
		return "(no matches)", nil
	}

	if len(matches) > grepMaxMatches {
		matches = matches[:grepMaxMatches]
	}

	result := strings.Join(matches, "\n")
	if len(result) > grepMaxResultSize {
		result = result[:grepMaxResultSize] + "\n... (truncated)"
	}
	if len(matches) >= grepMaxMatches {
		result += fmt.Sprintf("\n... (showing first %d matches)", grepMaxMatches)
	}

	return result, nil
}

func grepFile(path, needle string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var matches []string
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), needle) {
			display := line
			if len(display) > grepMaxLineLen {
				display = display[:grepMaxLineLen] + "..."
			}
			matches = append(matches, fmt.Sprintf("%s:%d: %s", path, lineNum, display))
			if len(matches) >= grepMaxMatches {
				break
			}
		}
	}
	return matches
}

package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Ls — list directory contents.
type Ls struct{}

func (t Ls) Name() string { return "ls" }
func (t Ls) Description() string {
	return "List files and directories in a given path. Returns names with type indicators (/ for dirs)."
}

func (t Ls) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Directory path to list (defaults to current dir)"
			}
		}
	}`)
}

func (t Ls) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	dir := params.Path
	if dir == "" {
		dir = "."
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("ls %s: %w", dir, err)
	}

	if len(entries) == 0 {
		return "(empty directory)", nil
	}

	sort.Slice(entries, func(i, j int) bool {
		// dirs first, then files
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	var sb strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		info, err := e.Info()
		if err == nil {
			sb.WriteString(fmt.Sprintf("%-40s %8d\n", name, info.Size()))
		} else {
			sb.WriteString(name + "\n")
		}
	}

	result := sb.String()
	if len(result) > 20000 {
		result = result[:20000] + "\n... (truncated)"
	}
	return strings.TrimSpace(result), nil
}

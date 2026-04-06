package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const execTimeout = 30 * time.Second

// Exec — тул для выполнения shell команд.
type Exec struct{}

func (t Exec) Name() string { return "exec" }
func (t Exec) Description() string {
	return "Execute a shell command and return its output (stdout + stderr). Timeout: 30 seconds."
}

func (t Exec) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "Shell command to execute"
			},
			"workdir": {
				"type": "string",
				"description": "Working directory (optional, defaults to current dir)"
			}
		},
		"required": ["command"]
	}`)
}

func (t Exec) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", params.Command)
	if params.Workdir != "" {
		cmd.Dir = params.Workdir
	}

	output, err := cmd.CombinedOutput()
	result := string(output)

	// Лимит вывода
	if len(result) > 20000 {
		result = result[:20000] + "\n... (truncated)"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result + "\n(timeout after 30s)", nil
		}
		return fmt.Sprintf("%s\nexit: %v", result, err), nil
	}

	if result == "" {
		result = "(no output)"
	}

	return strings.TrimSpace(result), nil
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const scriptTimeout = 30 * time.Second

// ScriptTool — внешний тул, определённый через manifest.json + исполняемый файл.
type ScriptTool struct {
	name        string
	description string
	parameters  json.RawMessage
	command     string // путь к исполняемому файлу
	workdir     string
}

// Manifest — описание внешнего тула.
type Manifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Command     string          `json:"command"` // относительно директории скилла
}

func (t *ScriptTool) Name() string               { return t.name }
func (t *ScriptTool) Description() string         { return t.description }
func (t *ScriptTool) Parameters() json.RawMessage { return t.parameters }

func (t *ScriptTool) Execute(args json.RawMessage) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.command)
	cmd.Dir = t.workdir
	cmd.Stdin = strings.NewReader(string(args))

	output, err := cmd.CombinedOutput()
	result := string(output)

	if len(result) > 20000 {
		result = result[:20000] + "\n... (truncated)"
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result + "\n(timeout)", nil
		}
		return fmt.Sprintf("%s\nerror: %v", result, err), nil
	}

	return strings.TrimSpace(result), nil
}

// LoadSkills сканирует директорию skills/ и регистрирует все найденные тулы.
func LoadSkills(dir string, registry *Registry) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}

	loaded := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		manifestPath := filepath.Join(skillDir, "manifest.json")

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			log.Printf("skills: invalid manifest in %s: %v", skillDir, err)
			continue
		}

		// Резолвим путь к команде
		cmdPath := filepath.Join(skillDir, manifest.Command)
		if _, err := os.Stat(cmdPath); err != nil {
			log.Printf("skills: command not found: %s", cmdPath)
			continue
		}

		tool := &ScriptTool{
			name:        manifest.Name,
			description: manifest.Description,
			parameters:  manifest.Parameters,
			command:     cmdPath,
			workdir:     skillDir,
		}

		registry.Register(tool)
		loaded++
		log.Printf("skills: loaded %q from %s", manifest.Name, skillDir)
	}

	return loaded
}

// ReloadSkills перезагружает скиллы (для hot reload).
// Удаляет старые ScriptTool из registry и загружает заново.
func ReloadSkills(dir string, registry *Registry) int {
	// Удаляем все ScriptTool
	registry.RemoveByType(func(t Tool) bool {
		_, ok := t.(*ScriptTool)
		return ok
	})
	return LoadSkills(dir, registry)
}

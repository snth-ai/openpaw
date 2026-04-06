package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CreateSkill — мета-тул: синт создаёт новый скилл (manifest + script).
// После создания скилл автоматически подхватывается при reload.
type CreateSkill struct {
	skillsDir string
	registry  *Registry
}

func NewCreateSkill(skillsDir string, registry *Registry) *CreateSkill {
	return &CreateSkill{skillsDir: skillsDir, registry: registry}
}

func (t *CreateSkill) Name() string { return "create_skill" }
func (t *CreateSkill) Description() string {
	return `Create a new skill (tool) that will be available for future use.
Write the manifest (name, description, parameters schema) and the executable script/binary.
The skill becomes available immediately after creation.
Use exec tool to compile Go code if needed (go build -o skills/<name>/run skills/<name>/main.go).`
}

func (t *CreateSkill) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Skill directory name (e.g., 'weather', 'translator')"
			},
			"tool_name": {
				"type": "string",
				"description": "Tool name as seen by LLM (e.g., 'get_weather')"
			},
			"description": {
				"type": "string",
				"description": "What the tool does"
			},
			"parameters_schema": {
				"type": "string",
				"description": "JSON Schema string for the tool parameters"
			},
			"script_content": {
				"type": "string",
				"description": "Content of the executable script (bash/python/etc). Receives JSON args on stdin, prints result to stdout."
			},
			"script_name": {
				"type": "string",
				"description": "Filename for the script (e.g., 'run.sh', 'run.py', 'main.go')"
			}
		},
		"required": ["name", "tool_name", "description", "parameters_schema", "script_content", "script_name"]
	}`)
}

func (t *CreateSkill) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Name             string `json:"name"`
		ToolName         string `json:"tool_name"`
		Description      string `json:"description"`
		ParametersSchema string `json:"parameters_schema"`
		ScriptContent    string `json:"script_content"`
		ScriptName       string `json:"script_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("parse args: %w", err)
	}

	skillDir := filepath.Join(t.skillsDir, params.Name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	// Пишем manifest.json
	manifest := Manifest{
		Name:        params.ToolName,
		Description: params.Description,
		Parameters:  json.RawMessage(params.ParametersSchema),
		Command:     params.ScriptName,
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(skillDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return "", fmt.Errorf("write manifest: %w", err)
	}

	// Пишем скрипт
	scriptPath := filepath.Join(skillDir, params.ScriptName)
	if err := os.WriteFile(scriptPath, []byte(params.ScriptContent), 0755); err != nil {
		return "", fmt.Errorf("write script: %w", err)
	}

	// Hot reload — подхватываем новый скилл
	loaded := ReloadSkills(t.skillsDir, t.registry)

	return fmt.Sprintf("Skill %q created in %s (%d skills total). If this is Go code, compile it with: exec {\"command\": \"go build -o %s/run %s/main.go\"}",
		params.ToolName, skillDir, loaded, skillDir, skillDir), nil
}

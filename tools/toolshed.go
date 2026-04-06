package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolShed — мета-тул, который выбирает какие тулы нужны для запроса.
// Получает от LLM список нужных тулов по их именам.
type ToolShed struct {
	registry *Registry
}

func NewToolShed(registry *Registry) *ToolShed {
	return &ToolShed{registry: registry}
}

// ToolSummary — компактное описание тула (name + description, без parameters).
type ToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Summaries возвращает компактные описания всех тулов (для первого LLM вызова).
func (ts *ToolShed) Summaries() []ToolSummary {
	var out []ToolSummary
	for _, t := range ts.registry.tools {
		// Пропускаем сам toolshed
		if t.Name() == "select_tools" {
			continue
		}
		out = append(out, ToolSummary{
			Name:        t.Name(),
			Description: t.Description(),
		})
	}
	return out
}

// SummariesPrompt форматирует список тулов для вставки в system prompt.
func (ts *ToolShed) SummariesPrompt() string {
	summaries := ts.Summaries()
	var sb strings.Builder
	sb.WriteString("Available tools (request by name if needed):\n")
	for _, s := range summaries {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", s.Name, s.Description))
	}
	return sb.String()
}

// ForLLMFiltered возвращает полные tool definitions только для запрошенных тулов.
func (ts *ToolShed) ForLLMFiltered(names []string) []map[string]any {
	var out []map[string]any
	for _, name := range names {
		t, ok := ts.registry.Get(name)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  json.RawMessage(t.Parameters()),
			},
		})
	}
	return out
}

// SelectToolsDef — определение мета-тула select_tools для первого вызова.
// LLM вызывает его чтобы запросить нужные тулы.
func SelectToolsDef() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "select_tools",
			"description": "Request specific tools to use. Call this with the names of tools you need. If you don't need any tools, just respond with text.",
			"parameters": json.RawMessage(`{
				"type": "object",
				"properties": {
					"tools": {
						"type": "array",
						"items": {"type": "string"},
						"description": "List of tool names you need for this response"
					}
				},
				"required": ["tools"]
			}`),
		},
	}
}

// ParseSelectedTools парсит ответ select_tools.
func ParseSelectedTools(args json.RawMessage) []string {
	var params struct {
		Tools []string `json:"tools"`
	}
	json.Unmarshal(args, &params)
	return params.Tools
}

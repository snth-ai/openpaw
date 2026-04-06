package tools

import (
	"encoding/json"
	"testing"
)

type mockTool struct {
	name        string
	description string
	params      json.RawMessage
	result      string
	err         error
}

func (m *mockTool) Name() string                { return m.name }
func (m *mockTool) Description() string         { return m.description }
func (m *mockTool) Parameters() json.RawMessage { return m.params }
func (m *mockTool) Execute(args json.RawMessage) (string, error) {
	return m.result, m.err
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool", description: "A test tool", params: json.RawMessage(`{}`)}

	r.Register(tool)

	if len(r.tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(r.tools))
	}

	if _, ok := r.tools["test_tool"]; !ok {
		t.Error("expected tool 'test_tool' to be registered")
	}

	tool2 := &mockTool{name: "another_tool", description: "Another tool", params: json.RawMessage(`{}`)}
	r.Register(tool2)

	if len(r.tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(r.tools))
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "get_tool", description: "Gettable tool", params: json.RawMessage(`{}`)}
	r.Register(tool)

	got, ok := r.Get("get_tool")
	if !ok {
		t.Error("expected to find 'get_tool'")
	}
	if got.Name() != "get_tool" {
		t.Errorf("expected name 'get_tool', got %s", got.Name())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected not to find 'nonexistent'")
	}
}

func TestRegistry_ForLLM(t *testing.T) {
	r := NewRegistry()

	r.Register(&mockTool{name: "zebra", description: "Z tool", params: json.RawMessage(`{"type":"object"}`)})
	r.Register(&mockTool{name: "alpha", description: "A tool", params: json.RawMessage(`{"type":"object"}`)})
	r.Register(&mockTool{name: "middle", description: "M tool", params: json.RawMessage(`{"type":"object"}`)})

	result := r.ForLLM()

	if len(result) != 3 {
		t.Fatalf("expected 3 tool descriptions, got %d", len(result))
	}

	if result[0]["function"].(map[string]any)["name"] != "alpha" {
		t.Errorf("expected first tool to be 'alpha', got %s", result[0]["function"].(map[string]any)["name"])
	}
	if result[1]["function"].(map[string]any)["name"] != "middle" {
		t.Errorf("expected second tool to be 'middle', got %s", result[1]["function"].(map[string]any)["name"])
	}
	if result[2]["function"].(map[string]any)["name"] != "zebra" {
		t.Errorf("expected third tool to be 'zebra', got %s", result[2]["function"].(map[string]any)["name"])
	}

	if _, hasCache := result[2]["cache_control"]; !hasCache {
		t.Error("expected last tool to have cache_control")
	}
	if _, hasCache := result[0]["cache_control"]; hasCache {
		t.Error("expected non-last tool to NOT have cache_control")
	}
}

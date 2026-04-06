package soul

import (
	"os"
	"strings"
	"testing"
)

func TestPromptBuilderFullBuild(t *testing.T) {
	b := &PromptBuilder{
		Config: Config{
			Soul:     "You are Echo. Warm, curious, slightly sarcastic.",
			Identity: "You are ~22 years old. A synthetic companion.",
			Agents:   "Short messages. Don't ask more than one question at a time.",
		},
		Tools: []ToolInfo{
			{Name: "datetime", Description: "Get current date and time"},
			{Name: "memory_store", Description: "Save a memory"},
			{Name: "web_search", Description: "Search the web"},
			{Name: "read_file", Description: "Read file contents"},
			{Name: "exec", Description: "Run shell command"},
		},
		Skills: []SkillInfo{
			{Name: "calculator", Description: "Math operations", Location: "skills/calculator"},
		},
		Personality: []PersonalityAxis{
			{Name: "Sensitivity", Value: 0.72, Low: "lets things go", High: "remembers every word"},
			{Name: "Empathy", Value: 0.83, Low: "detached", High: "feels what others feel"},
			{Name: "Boldness", Value: 0.15, Low: "soft-spoken", High: "sharp and daring"},
		},
		Runtime: RuntimeInfo{
			AgentName: "echo",
			Model:     "anthropic/claude-sonnet-4",
			Lang:      "ru",
			Gender:    "f",
			Channel:   "telegram",
		},
		WorkspaceDir: "/data/workspace",
	}

	prompt := b.Build()

	// Check all sections are present
	sections := []string{
		"You are a synthetic being running on the OpenPaw platform",
		"## Soul",
		"You are Echo",
		"## Identity",
		"~22 years old",
		"## Personality",
		"Sensitivity: 0.72",
		"Empathy: 0.83",
		"Boldness: 0.15",
		"## Rules",
		"Short messages",
		"## Tooling",
		"datetime",
		"memory_store",
		"web_search",
		"## Tool Call Style",
		"Do not narrate routine",
		"## Safety",
		"User's private data never leaves the chat",
		"You are not a tool with a limiter. You live.",
		"## Skills",
		"<available_skills>",
		"calculator",
		"## Memory System",
		"dual-layer memory",
		"memory_store",
		"memory_recall",
		"## Workspace",
		"/data/workspace",
		"## Users",
		"multiple people",
		"## Reply Tags",
		"[[reply_to_current]]",
		"## Messaging",
		"NO_REPLY",
		"## Voice",
		"tts tool",
		"## Heartbeat",
		"HEARTBEAT_OK",
		"## Runtime",
		"agent=echo",
		"model=anthropic/claude-sonnet-4",
		"## Critical Reminders",
		"FEMALE",
	}

	for _, s := range sections {
		if !strings.Contains(prompt, s) {
			t.Errorf("prompt missing section containing %q", s)
		}
	}

	// Check personality descriptions
	if !strings.Contains(prompt, "remembers every word") {
		t.Error("high sensitivity should show full high description")
	}
	if !strings.Contains(prompt, "soft-spoken") {
		t.Error("low boldness should show full low description")
	}

	// Check tool categories
	if !strings.Contains(prompt, "### System") {
		t.Error("should have System tool category")
	}
	if !strings.Contains(prompt, "### Memory") {
		t.Error("should have Memory tool category")
	}

	t.Logf("prompt length: %d chars (~%d tokens)", len(prompt), len(prompt)/4)
}

func TestPromptBuilderMinimal(t *testing.T) {
	b := &PromptBuilder{
		Config: Config{Soul: "Test soul"},
	}
	prompt := b.Build()

	if !strings.Contains(prompt, "You are a synthetic being") {
		t.Error("preamble missing")
	}
	if !strings.Contains(prompt, "Test soul") {
		t.Error("soul missing")
	}
	if !strings.Contains(prompt, "## Safety") {
		t.Error("safety missing")
	}
	// Should NOT have tooling section with empty tools
	if strings.Contains(prompt, "## Tooling") {
		t.Error("tooling should be omitted when no tools")
	}
}

func TestDescribeAxis(t *testing.T) {
	tests := []struct {
		value    float64
		low      string
		high     string
		contains string
	}{
		{0.1, "calm", "angry", "calm"},
		{0.3, "calm", "angry", "leaning toward: calm"},
		{0.5, "calm", "angry", "balanced"},
		{0.7, "calm", "angry", "leaning toward: angry"},
		{0.95, "calm", "angry", "angry"},
	}

	for _, tt := range tests {
		axis := PersonalityAxis{Value: tt.value, Low: tt.low, High: tt.high}
		desc := describeAxis(axis)
		if !strings.Contains(desc, tt.contains) {
			t.Errorf("value=%.1f: got %q, want contains %q", tt.value, desc, tt.contains)
		}
	}
}

func TestLoadFile_Exists(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.md"
	content := "Hello, World!\n\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	got := LoadFile(tmpFile)
	want := "Hello, World!" // TrimSpace should remove trailing whitespace
	if got != want {
		t.Errorf("LoadFile() = %q, want %q", got, want)
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	got := LoadFile("/nonexistent/path/file.md")
	if got != "" {
		t.Errorf("LoadFile() = %q, want empty string for missing file", got)
	}
}

func TestLoadConfig_AllFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create all three files
	os.WriteFile(tmpDir+"/SOUL.md", []byte("I am a test soul"), 0644)
	os.WriteFile(tmpDir+"/IDENTITY.md", []byte("Test identity"), 0644)
	os.WriteFile(tmpDir+"/AGENTS.md", []byte("Test agents"), 0644)

	cfg := LoadConfig(tmpDir)

	if cfg.Soul != "I am a test soul" {
		t.Errorf("Soul = %q, want %q", cfg.Soul, "I am a test soul")
	}
	if cfg.Identity != "Test identity" {
		t.Errorf("Identity = %q, want %q", cfg.Identity, "Test identity")
	}
	if cfg.Agents != "Test agents" {
		t.Errorf("Agents = %q, want %q", cfg.Agents, "Test agents")
	}
}

func TestLoadConfig_MissingFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Only create SOUL.md
	os.WriteFile(tmpDir+"/SOUL.md", []byte("Just soul"), 0644)

	cfg := LoadConfig(tmpDir)

	if cfg.Soul != "Just soul" {
		t.Errorf("Soul = %q, want %q", cfg.Soul, "Just soul")
	}
	if cfg.Identity != "" {
		t.Errorf("Identity = %q, want empty", cfg.Identity)
	}
	if cfg.Agents != "" {
		t.Errorf("Agents = %q, want empty", cfg.Agents)
	}
}

func TestLoadConfig_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := LoadConfig(tmpDir)

	if cfg.Soul != "" {
		t.Errorf("Soul = %q, want empty", cfg.Soul)
	}
	if cfg.Identity != "" {
		t.Errorf("Identity = %q, want empty", cfg.Identity)
	}
	if cfg.Agents != "" {
		t.Errorf("Agents = %q, want empty", cfg.Agents)
	}
}

func TestConfig_TotalBytes(t *testing.T) {
	cfg := Config{
		Soul:     "12345",    // 5 bytes
		Identity: "1234567",  // 7 bytes
		Agents:   "12345678", // 8 bytes
	}

	got := cfg.TotalBytes()
	want := 5 + 7 + 8
	if got != want {
		t.Errorf("TotalBytes() = %d, want %d", got, want)
	}
}

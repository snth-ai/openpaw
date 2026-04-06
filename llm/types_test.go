package llm

import (
	"encoding/json"
	"testing"
)

func TestMessage_StringContent(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "Hello, world!",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded.Role != msg.Role {
		t.Errorf("role: got %q, want %q", decoded.Role, msg.Role)
	}
	if decoded.Content != msg.Content {
		t.Errorf("content: got %q, want %q", decoded.Content, msg.Content)
	}
	if len(decoded.ContentParts) > 0 {
		t.Errorf("expected no ContentParts, got %d", len(decoded.ContentParts))
	}
}

func TestMessage_ArrayContent(t *testing.T) {
	msg := Message{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: "Hello", CacheControl: &CacheControlPart{Type: "ephemeral"}},
			{Type: "text", Text: " world"},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded.Role != msg.Role {
		t.Errorf("role: got %q, want %q", decoded.Role, msg.Role)
	}

	expectedContent := "Hello world"
	if decoded.Content != expectedContent {
		t.Errorf("content: got %q, want %q", decoded.Content, expectedContent)
	}

	if len(decoded.ContentParts) != len(msg.ContentParts) {
		t.Errorf("contentParts length: got %d, want %d", len(decoded.ContentParts), len(msg.ContentParts))
	}

	if len(decoded.ContentParts) > 0 {
		if decoded.ContentParts[0].Text != "Hello" {
			t.Errorf("first part text: got %q, want %q", decoded.ContentParts[0].Text, "Hello")
		}
		if decoded.ContentParts[0].CacheControl == nil {
			t.Error("expected CacheControl on first part, got nil")
		} else if decoded.ContentParts[0].CacheControl.Type != "ephemeral" {
			t.Errorf("CacheControl type: got %q, want %q", decoded.ContentParts[0].CacheControl.Type, "ephemeral")
		}
		if decoded.ContentParts[1].Text != " world" {
			t.Errorf("second part text: got %q, want %q", decoded.ContentParts[1].Text, " world")
		}
	}
}

func TestMessage_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		message Message
	}{
		{
			name: "simple string content",
			message: Message{
				Role:    "assistant",
				Content: "Hi there!",
			},
		},
		{
			name: "block content with cache control",
			message: Message{
				Role: "user",
				ContentParts: []ContentPart{
					{Type: "text", Text: "Part 1", CacheControl: &CacheControlPart{Type: "ephemeral"}},
					{Type: "text", Text: "Part 2"},
				},
			},
		},
		{
			name: "with tool calls",
			message: Message{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location":"NYC"}`,
						},
					},
				},
			},
		},
		{
			name: "tool response",
			message: Message{
				Role:       "tool",
				Content:    `{"temp": 72}`,
				ToolCallID: "call_123",
			},
		},
		{
			name: "empty message",
			message: Message{
				Role: "system",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.message)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded Message
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

		if decoded.Role != tt.message.Role {
			t.Errorf("role: got %q, want %q", decoded.Role, tt.message.Role)
		}

		// For ContentParts messages, Content is derived from parts during unmarshal
		if len(tt.message.ContentParts) == 0 && decoded.Content != tt.message.Content {
			t.Errorf("content: got %q, want %q", decoded.Content, tt.message.Content)
		}

		if decoded.ToolCallID != tt.message.ToolCallID {
				t.Errorf("toolCallID: got %q, want %q", decoded.ToolCallID, tt.message.ToolCallID)
			}

			if len(decoded.ToolCalls) != len(tt.message.ToolCalls) {
				t.Errorf("toolCalls length: got %d, want %d", len(decoded.ToolCalls), len(tt.message.ToolCalls))
			} else {
				for i, tc := range decoded.ToolCalls {
					if tc.ID != tt.message.ToolCalls[i].ID {
						t.Errorf("toolCall[%d].ID: got %q, want %q", i, tc.ID, tt.message.ToolCalls[i].ID)
					}
					if tc.Type != tt.message.ToolCalls[i].Type {
						t.Errorf("toolCall[%d].Type: got %q, want %q", i, tc.Type, tt.message.ToolCalls[i].Type)
					}
					if tc.Function.Name != tt.message.ToolCalls[i].Function.Name {
						t.Errorf("toolCall[%d].Function.Name: got %q, want %q", i, tc.Function.Name, tt.message.ToolCalls[i].Function.Name)
					}
					if tc.Function.Arguments != tt.message.ToolCalls[i].Function.Arguments {
						t.Errorf("toolCall[%d].Function.Arguments: got %q, want %q", i, tc.Function.Arguments, tt.message.ToolCalls[i].Function.Arguments)
					}
				}
			}
		})
	}
}

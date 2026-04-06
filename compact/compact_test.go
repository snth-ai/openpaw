package compact

import (
	"testing"

	"github.com/openpaw/server/llm"
)

func TestNeedsCompact_DetectsOversized(t *testing.T) {
	tests := []struct {
		name      string
		charCount int
		want      bool
	}{
		{
			name:      "empty",
			charCount: 0,
			want:      false,
		},
		{
			name:      "small",
			charCount: 1000,
			want:      false,
		},
		{
			name:      "medium",
			charCount: 50000,
			want:      false,
		},
		{
			name:      "large",
			charCount: 150000,
			want:      true,
		},
		{
			name:      "very large",
			charCount: 300000,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := []llm.Message{
				{Role: "user", Content: string(make([]byte, tt.charCount))},
			}
			got := NeedsCompact(msgs)
			if got != tt.want {
				t.Errorf("NeedsCompact() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsCompact_SmallMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}

	got := NeedsCompact(msgs)
	if got {
		t.Error("NeedsCompact() should return false for small messages")
	}
}

func TestNeedsCompact_ManySmallMessages(t *testing.T) {
	msgs := make([]llm.Message, 50)
	for i := range msgs {
		msgs[i] = llm.Message{
			Role:    "user",
			Content: "short message",
		}
	}

	got := NeedsCompact(msgs)
	if got {
		t.Error("NeedsCompact() should return false for many small messages under threshold")
	}
}

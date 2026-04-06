package personality

import (
	"encoding/json"
	"testing"
)

func TestPersonality_ClampTo01(t *testing.T) {
	for i := 0; i < 100; i++ {
		m := Generate()
		axes := []float64{
			m.Sensitivity, m.Sexuality, m.Openness, m.Attachment,
			m.Boldness, m.Humor, m.Jealousy, m.Romanticism,
			m.Stubbornness, m.Empathy,
		}
		for _, v := range axes {
			if v < 0 || v >= 1 {
				t.Errorf("axis value %f out of [0,1) range", v)
			}
		}
	}
}

func TestPersonality_DecayMultiplier(t *testing.T) {
	tests := []struct {
		sensitivity float64
		want        float64
	}{
		{0.0, 1.5},
		{0.1, 1.4},
		{0.5, 1.0},
		{0.9, 0.6},
		{1.0, 0.5},
	}
	for _, tt := range tests {
		m := Matrix{Sensitivity: tt.sensitivity}
		got := m.DecayMultiplier()
		if got != tt.want {
			t.Errorf("DecayMultiplier(sensitivity=%.1f) = %f, want %f", tt.sensitivity, got, tt.want)
		}
	}
}

func TestPersonality_RoundTrip(t *testing.T) {
	orig := Matrix{
		Sensitivity:  0.7,
		Sexuality:    0.3,
		Openness:     0.5,
		Attachment:   0.8,
		Boldness:     0.2,
		Humor:        0.6,
		Jealousy:     0.4,
		Romanticism:  0.9,
		Stubbornness: 0.1,
		Empathy:      0.75,
	}
	jsonStr := orig.JSON()
	var decoded Matrix
	if err := json.Unmarshal([]byte(jsonStr), &decoded); err != nil {
		t.Fatalf("JSON round-trip failed: %v", err)
	}
	if decoded != orig {
		t.Errorf("decoded matrix != original\ngot:  %+v\nwant: %+v", decoded, orig)
	}
}

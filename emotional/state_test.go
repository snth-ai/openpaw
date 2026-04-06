package emotional

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestNewState(t *testing.T) {
	s := NewState()

	if s.Desire != 0.0 {
		t.Errorf("Desire = %v, want 0.0", s.Desire)
	}
	if s.Warmth != 0.3 {
		t.Errorf("Warmth = %v, want 0.3", s.Warmth)
	}
	if s.Hurt != 0.0 {
		t.Errorf("Hurt = %v, want 0.0", s.Hurt)
	}
	if s.Frustration != 0.0 {
		t.Errorf("Frustration = %v, want 0.0", s.Frustration)
	}
	if s.Joy != 0.3 {
		t.Errorf("Joy = %v, want 0.3", s.Joy)
	}
	if s.Trust != 0.5 {
		t.Errorf("Trust = %v, want 0.5", s.Trust)
	}
	if len(s.Scars) != 0 {
		t.Errorf("Scars length = %v, want 0", len(s.Scars))
	}
	if s.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set")
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{-0.5, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{1.5, 1},
		{2.0, 1},
	}

	for _, tt := range tests {
		got := clamp01(tt.input)
		if got != tt.want {
			t.Errorf("clamp01(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestStateClamping(t *testing.T) {
	s := NewState()

	// Apply many positive triggers to push values above 1
	for i := 0; i < 20; i++ {
		s.Apply(TriggerIntimacyGentle)
	}

	if s.Warmth > 1.0 {
		t.Errorf("Warmth = %v, should be <= 1.0", s.Warmth)
	}
	if s.Trust > 1.0 {
		t.Errorf("Trust = %v, should be <= 1.0", s.Trust)
	}
	if s.Joy > 1.0 {
		t.Errorf("Joy = %v, should be <= 1.0", s.Joy)
	}
	if s.Desire > 1.0 {
		t.Errorf("Desire = %v, should be <= 1.0", s.Desire)
	}
}

func TestClampingNegative(t *testing.T) {
	s := NewState()

	// Apply betrayal to max hurt, then many apologies to push below 0
	s.Apply(TriggerBetrayal)
	for i := 0; i < 20; i++ {
		s.Apply(TriggerApology)
	}

	if s.Hurt < 0 {
		t.Errorf("Hurt = %v, should be >= 0", s.Hurt)
	}
	if s.Frustration < 0 {
		t.Errorf("Frustration = %v, should be >= 0", s.Frustration)
	}
}

func TestCompactJSON(t *testing.T) {
	s := NewState()
	s.Apply(TriggerCompliment)

	out := s.CompactJSON()

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("CompactJSON produced invalid JSON: %v", err)
	}

	expected := []string{"desire", "warmth", "hurt", "frustration", "joy", "trust"}
	for _, key := range expected {
		if _, ok := m[key]; !ok {
			t.Errorf("CompactJSON missing key %q", key)
		}
	}

	// No scars yet, so scars_count should not be present
	if _, ok := m["scars_count"]; ok {
		t.Error("CompactJSON should not have scars_count when no scars")
	}
}

func TestCompactJSONWithScars(t *testing.T) {
	s := NewState()
	s.Apply(TriggerBetrayal)

	out := s.CompactJSON()

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("CompactJSON produced invalid JSON: %v", err)
	}

	if _, ok := m["scars_count"]; !ok {
		t.Error("CompactJSON should have scars_count when scars exist")
	}
	if _, ok := m["scars_weight"]; !ok {
		t.Error("CompactJSON should have scars_weight when scars exist")
	}
}

func TestFullJSON(t *testing.T) {
	s := NewState()
	s.Apply(TriggerFlirt)

	out := s.FullJSON()

	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("FullJSON produced invalid JSON: %v", err)
	}

	expected := []string{"desire", "warmth", "hurt", "frustration", "joy", "trust", "scars", "last_updated"}
	for _, key := range expected {
		if _, ok := m[key]; !ok {
			t.Errorf("FullJSON missing key %q", key)
		}
	}
}

func TestFullJSONRoundTrip(t *testing.T) {
	s := NewState()
	s.Apply(TriggerGift)
	s.Apply(TriggerBetrayal)

	out := s.FullJSON()

	var restored State
	if err := json.Unmarshal([]byte(out), &restored); err != nil {
		t.Fatalf("FullJSON round-trip failed: %v", err)
	}

	if restored.Desire != s.Desire {
		t.Errorf("Desire mismatch: got %v, want %v", restored.Desire, s.Desire)
	}
	if restored.Warmth != s.Warmth {
		t.Errorf("Warmth mismatch: got %v, want %v", restored.Warmth, s.Warmth)
	}
	if len(restored.Scars) != len(s.Scars) {
		t.Errorf("Scars length mismatch: got %v, want %v", len(restored.Scars), len(s.Scars))
	}
}

func TestTriggerApply_Joy(t *testing.T) {
	s := NewState()
	beforeJoy := s.Joy

	s.Apply(TriggerJoke)

	if s.Joy <= beforeJoy {
		t.Errorf("TriggerJoke should increase Joy: got %v, was %v", s.Joy, beforeJoy)
	}
	if s.Warmth <= 0.3 {
		t.Errorf("TriggerJoke should increase Warmth: got %v", s.Warmth)
	}
}

func TestTriggerApply_Sadness(t *testing.T) {
	s := NewState()

	s.Apply(TriggerCriticism)

	if s.Hurt < 0.1 {
		t.Errorf("TriggerCriticism should increase Hurt: got %v", s.Hurt)
	}
	if s.Frustration < 0.1 {
		t.Errorf("TriggerCriticism should increase Frustration: got %v", s.Frustration)
	}
}

func TestTriggerApply_Betrayal(t *testing.T) {
	s := NewState()

	s.Apply(TriggerBetrayal)

	if s.Trust > 0.0 {
		t.Errorf("TriggerBetrayal should reduce Trust below 0: got %v", s.Trust)
	}
	if s.Hurt != 1.0 {
		t.Errorf("TriggerBetrayal should set Hurt to 1.0: got %v", s.Hurt)
	}
	if len(s.Scars) != 1 {
		t.Fatalf("TriggerBetrayal should add 1 scar: got %d", len(s.Scars))
	}
	if s.Scars[0].Cause != "betrayal" {
		t.Errorf("Scar cause should be 'betrayal': got %q", s.Scars[0].Cause)
	}
	if s.Scars[0].Intensity != 1.0 {
		t.Errorf("Scar intensity should be 1.0: got %v", s.Scars[0].Intensity)
	}
}

func TestTriggerApply_Multiple(t *testing.T) {
	s := NewState()

	s.Apply(TriggerCompliment)
	s.Apply(TriggerFlirt)
	s.Apply(TriggerGift)

	if s.Joy < 0.3 {
		t.Errorf("Multiple positive triggers should result in notable Joy: got %v", s.Joy)
	}
	if s.Warmth < 0.3 {
		t.Errorf("Multiple positive triggers should result in notable Warmth: got %v", s.Warmth)
	}
	if s.Desire < 0.2 {
		t.Errorf("Multiple positive triggers should result in notable Desire: got %v", s.Desire)
	}
}

func TestTriggerApply_UnknownIgnored(t *testing.T) {
	s := NewState()
	beforeDesire := s.Desire
	beforeWarmth := s.Warmth
	beforeHurt := s.Hurt
	beforeFrustration := s.Frustration
	beforeJoy := s.Joy
	beforeTrust := s.Trust

	// Apply an unknown trigger value (beyond defined triggers)
	unknownTrigger := Trigger(999)
	s.Apply(unknownTrigger)

	if s.Desire != beforeDesire {
		t.Errorf("Unknown trigger should not change Desire: got %v, was %v", s.Desire, beforeDesire)
	}
	if s.Warmth != beforeWarmth {
		t.Errorf("Unknown trigger should not change Warmth: got %v, was %v", s.Warmth, beforeWarmth)
	}
	if s.Hurt != beforeHurt {
		t.Errorf("Unknown trigger should not change Hurt: got %v, was %v", s.Hurt, beforeHurt)
	}
	if s.Frustration != beforeFrustration {
		t.Errorf("Unknown trigger should not change Frustration: got %v, was %v", s.Frustration, beforeFrustration)
	}
	if s.Joy != beforeJoy {
		t.Errorf("Unknown trigger should not change Joy: got %v, was %v", s.Joy, beforeJoy)
	}
	if s.Trust != beforeTrust {
		t.Errorf("Unknown trigger should not change Trust: got %v, was %v", s.Trust, beforeTrust)
	}
}

func TestDecay_ReducesIntensity(t *testing.T) {
	s := NewState()
	s.Apply(TriggerJoke)
	highJoy := s.Joy

	// Simulate time passing
	s.LastUpdated = time.Now().Add(-10 * time.Hour)
	s.Decay(1.0)

	if s.Joy >= highJoy {
		t.Errorf("Decay should reduce Joy: got %v, was %v", s.Joy, highJoy)
	}
}

func TestDecay_ScarsHaveFloor(t *testing.T) {
	s := NewState()
	s.Apply(TriggerBetrayal)

	// Simulate a very long time passing
	s.LastUpdated = time.Now().Add(-10000 * time.Hour)
	s.Decay(1.0)

	if len(s.Scars) == 0 {
		t.Fatal("Scars should still exist after decay")
	}
	if s.Scars[0].Intensity < 0.05 {
		t.Errorf("Scar intensity should not go below 0.05: got %v", s.Scars[0].Intensity)
	}
	if math.Abs(s.Scars[0].Intensity-0.05) > 0.001 {
		t.Errorf("Scar intensity should be at floor 0.05: got %v", s.Scars[0].Intensity)
	}
}

func TestDecay_ZeroElapsed(t *testing.T) {
	s := NewState()
	s.Apply(TriggerJoke)
	beforeJoy := s.Joy
	beforeWarmth := s.Warmth
	beforeHurt := s.Hurt

	// Set LastUpdated to now — no time has elapsed
	s.LastUpdated = time.Now()
	s.Decay(1.0)

	if s.Joy != beforeJoy {
		t.Errorf("Decay with zero elapsed time should not change Joy: got %v, was %v", s.Joy, beforeJoy)
	}
	if s.Warmth != beforeWarmth {
		t.Errorf("Decay with zero elapsed time should not change Warmth: got %v, was %v", s.Warmth, beforeWarmth)
	}
	if s.Hurt != beforeHurt {
		t.Errorf("Decay with zero elapsed time should not change Hurt: got %v, was %v", s.Hurt, beforeHurt)
	}
}

func TestDecay_Deterministic(t *testing.T) {
	s1 := NewState()
	s2 := NewState()

	s1.Apply(TriggerGift)
	s2.Apply(TriggerGift)

	// Set same LastUpdated for both
	past := time.Now().Add(-24 * time.Hour)
	s1.LastUpdated = past
	s2.LastUpdated = past

	s1.Decay(1.0)
	s2.Decay(1.0)

	if math.Abs(s1.Joy-s2.Joy) > 0.0001 {
		t.Errorf("Decay should be deterministic: s1.Joy=%v, s2.Joy=%v", s1.Joy, s2.Joy)
	}
	if math.Abs(s1.Warmth-s2.Warmth) > 0.0001 {
		t.Errorf("Decay should be deterministic: s1.Warmth=%v, s2.Warmth=%v", s1.Warmth, s2.Warmth)
	}
	if math.Abs(s1.Trust-s2.Trust) > 0.0001 {
		t.Errorf("Decay should be deterministic: s1.Trust=%v, s2.Trust=%v", s1.Trust, s2.Trust)
	}
}

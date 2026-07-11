package memory

import (
	"math"
	"testing"
	"time"
)

// Decay задокументирован как «importance *= factor per day». Часовой тик
// обязан применять дробную долю дневного фактора, а не целый день —
// иначе память декэится в ~24 раза быстрее (0.995 в час вместо в день).
func TestApplyDecayHourlyTickIsFractional(t *testing.T) {
	m := Memory{Importance: 0.9, UpdatedAt: time.Now().Add(-time.Hour)}
	ApplyDecay(&m, DefaultDecayConfig())

	want := 0.9 * math.Pow(DefaultDecayFactor, 1.0/24)
	if math.Abs(m.Importance-want) > 0.001 {
		t.Fatalf("hourly decay: importance=%.5f, want ~%.5f (fractional day), full-day factor would give %.5f",
			m.Importance, want, 0.9*DefaultDecayFactor)
	}
}

// 24 часовых тика подряд должны компаундиться ровно в один дневной фактор.
func TestApplyDecayCompoundsToDailyRate(t *testing.T) {
	m := Memory{Importance: 0.9}
	for i := 0; i < 24; i++ {
		m.UpdatedAt = time.Now().Add(-time.Hour)
		ApplyDecay(&m, DefaultDecayConfig())
	}

	want := 0.9 * DefaultDecayFactor
	if math.Abs(m.Importance-want) > 0.005 {
		t.Fatalf("24 hourly ticks: importance=%.5f, want ~%.5f (one daily factor)", m.Importance, want)
	}
}

// ApplyDecay обязан сбрасывать decay-часы: без этого повторный прогон
// по тому же UpdatedAt применяет фактор ещё раз (double-decay; так текла
// память у FileStore, который не переписывал UpdatedAt сам).
func TestApplyDecayResetsClock(t *testing.T) {
	m := Memory{Importance: 0.9, UpdatedAt: time.Now().Add(-48 * time.Hour)}
	ApplyDecay(&m, DefaultDecayConfig())
	first := m.Importance

	ApplyDecay(&m, DefaultDecayConfig())
	if m.Importance != first {
		t.Fatalf("second immediate ApplyDecay changed importance %.5f -> %.5f (clock not reset)", first, m.Importance)
	}
}

package memory

import "time"

const (
	DefaultDecayFactor = 0.995 // importance *= factor per day
	DefaultMinImportance = 0.05 // ниже — удаляем
	AccessBoost        = 0.02  // бонус при каждом access
	MaxImportance      = 1.0
)

// DecayConfig — настройки maintenance.
type DecayConfig struct {
	DecayFactor    float64 // множитель importance за день (0.99 = медленный, 0.9 = быстрый)
	MinImportance  float64 // порог для удаления
	AccessBoost    float64 // бонус при recall
}

func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		DecayFactor:   DefaultDecayFactor,
		MinImportance: DefaultMinImportance,
		AccessBoost:   AccessBoost,
	}
}

// ApplyDecay применяет decay к importance на основе времени с последнего обновления.
func ApplyDecay(m *Memory, cfg DecayConfig) {
	days := time.Since(m.UpdatedAt).Hours() / 24
	if days < 0.01 {
		return
	}
	// importance *= decay_factor ^ days
	factor := 1.0
	for d := 0.0; d < days; d += 1.0 {
		factor *= cfg.DecayFactor
	}
	m.Importance *= factor
	if m.Importance < cfg.MinImportance {
		m.Importance = 0
	}
}

// BoostAccess увеличивает importance при recall.
func BoostAccess(m *Memory, cfg DecayConfig) {
	m.AccessCount++
	m.LastAccess = time.Now()
	m.Importance += cfg.AccessBoost
	if m.Importance > MaxImportance {
		m.Importance = MaxImportance
	}
}

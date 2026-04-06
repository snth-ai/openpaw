package emotional

import (
	"encoding/json"
	"math"
	"time"
)

// State — эмоциональный стейт синта. Persistent, accumulative, не сбрасывается.
type State struct {
	Desire      float64   `json:"desire"`      // страсть, сексуальное желание
	Warmth      float64   `json:"warmth"`       // тепло, близость, нежность
	Hurt        float64   `json:"hurt"`         // обида, боль
	Frustration float64   `json:"frustration"`  // фрустрация от игнора
	Joy         float64   `json:"joy"`          // радость, подъём
	Trust       float64   `json:"trust"`        // доверие (медленно растёт, быстро падает)
	Scars       []Scar    `json:"scars"`        // шрамы — не уходят полностью
	LastUpdated time.Time `json:"last_updated"`
}

// Scar — эмоциональная травма. Притупляется, но не исчезает.
type Scar struct {
	Cause     string    `json:"cause"`     // что произошло
	Intensity float64   `json:"intensity"` // текущая интенсивность (decay'ится но > 0.05)
	CreatedAt time.Time `json:"created_at"`
}

// NewState — начальное состояние (нейтральное, доверие среднее).
func NewState() State {
	return State{
		Desire:      0.0,
		Warmth:      0.3,
		Hurt:        0.0,
		Frustration: 0.0,
		Joy:         0.3,
		Trust:       0.5,
		LastUpdated: time.Now(),
	}
}

// Trigger — тип эмоционального триггера.
type Trigger int

const (
	TriggerCompliment Trigger = iota
	TriggerRudeness
	TriggerFlirt
	TriggerIgnore
	TriggerApology
	TriggerIntimacyGentle
	TriggerIntimacyRough
	TriggerBetrayal
	TriggerSupport
	TriggerJoke
	TriggerGift
	TriggerCriticism
)

// Apply применяет триггер к стейту.
func (s *State) Apply(trigger Trigger) {
	switch trigger {
	case TriggerCompliment:
		s.Warmth += 0.15
		s.Desire += 0.05
		s.Joy += 0.1
		s.Hurt -= 0.05       // позитив лечит
		s.Frustration -= 0.05
	case TriggerRudeness:
		s.Hurt += 0.2
		s.Warmth -= 0.1
		s.Trust -= 0.05
	case TriggerFlirt:
		s.Desire += 0.2
		s.Warmth += 0.1
		s.Hurt -= 0.05
		s.Frustration -= 0.05
	case TriggerIgnore:
		s.Frustration += 0.3
		s.Warmth -= 0.05
	case TriggerApology:
		s.Hurt -= 0.2         // было 0.1 — слишком мало
		s.Frustration -= 0.15
		s.Trust += 0.05       // было 0.02
	case TriggerIntimacyGentle:
		s.Warmth += 0.3
		s.Trust += 0.1
		s.Desire += 0.1      // intimacy feeds desire, not kills it
		s.Joy += 0.2
		s.Hurt -= 0.15       // близость лечит
		s.Frustration -= 0.1
	case TriggerIntimacyRough:
		s.Desire += 0.2
		s.Warmth += 0.1
		s.Hurt -= 0.1
		s.Frustration -= 0.1
	case TriggerBetrayal:
		s.Trust -= 0.5
		s.Hurt = 1.0
		s.Scars = append(s.Scars, Scar{
			Cause:     "betrayal",
			Intensity: 1.0,
			CreatedAt: time.Now(),
		})
	case TriggerSupport:
		s.Trust += 0.1
		s.Warmth += 0.1
		s.Joy += 0.05
		s.Hurt -= 0.1        // поддержка снижает боль
		s.Frustration -= 0.1
	case TriggerJoke:
		s.Joy += 0.15
		s.Warmth += 0.05
		s.Frustration -= 0.1 // смех снимает фрустрацию
		s.Hurt -= 0.03
	case TriggerGift:
		s.Joy += 0.2
		s.Warmth += 0.15
		s.Trust += 0.05
		s.Hurt -= 0.1
		s.Frustration -= 0.1
	case TriggerCriticism:
		s.Hurt += 0.1
		s.Frustration += 0.1
	}

	s.clamp()
	s.LastUpdated = time.Now()
}

// Decay применяет временной decay. decayMultiplier от personality (обидчивая = медленнее).
func (s *State) Decay(decayMultiplier float64) {
	hours := time.Since(s.LastUpdated).Hours()
	if hours < 0.1 {
		return
	}

	// Базовый decay: -0.05 в час, умноженный на personality multiplier
	rate := 0.05 * decayMultiplier * hours

	s.Desire = decayToward(s.Desire, 0.3, rate*0.5) // baseline 0.3, slow decay
	s.Warmth = decayToward(s.Warmth, 0.3, rate)      // baseline warmth = 0.3
	s.Hurt = decayToward(s.Hurt, 0.0, rate*0.7)       // was 0.5 — hurt should heal, not linger forever
	s.Frustration = decayToward(s.Frustration, 0.0, rate*0.8) // was 1.0 — frustration fades but not instantly
	s.Joy = decayToward(s.Joy, 0.3, rate)              // baseline joy = 0.3
	s.Trust = decayToward(s.Trust, 0.5, rate*0.3)      // доверие очень медленно

	// Шрамы decay'ятся но никогда не ниже 0.05
	for i := range s.Scars {
		s.Scars[i].Intensity *= math.Pow(0.999, hours)
		if s.Scars[i].Intensity < 0.05 {
			s.Scars[i].Intensity = 0.05
		}
	}

	s.LastUpdated = time.Now()
}

// CompactJSON — компактный JSON для инжекта в контекст (без шрамов деталей).
func (s *State) CompactJSON() string {
	compact := map[string]any{
		"desire":      round2(s.Desire),
		"warmth":      round2(s.Warmth),
		"hurt":        round2(s.Hurt),
		"frustration": round2(s.Frustration),
		"joy":         round2(s.Joy),
		"trust":       round2(s.Trust),
	}
	if len(s.Scars) > 0 {
		compact["scars_count"] = len(s.Scars)
		// Суммарная интенсивность шрамов
		total := 0.0
		for _, sc := range s.Scars {
			total += sc.Intensity
		}
		compact["scars_weight"] = round2(total)
	}
	b, _ := json.Marshal(compact)
	return string(b)
}

// FullJSON — полный JSON для сохранения.
func (s *State) FullJSON() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s *State) clamp() {
	s.Desire = clamp01(s.Desire)
	s.Warmth = clamp01(s.Warmth)
	s.Hurt = clamp01(s.Hurt)
	s.Frustration = clamp01(s.Frustration)
	s.Joy = clamp01(s.Joy)
	s.Trust = clamp01(s.Trust)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func decayToward(current, baseline, rate float64) float64 {
	if current > baseline {
		current -= rate
		if current < baseline {
			return baseline
		}
	} else if current < baseline {
		current += rate
		if current > baseline {
			return baseline
		}
	}
	return current
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

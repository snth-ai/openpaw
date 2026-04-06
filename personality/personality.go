package personality

import (
	"encoding/json"
	"math/rand"
)

// Matrix — 10 осей характера, рандом при создании, неизменяемый базовый темперамент.
type Matrix struct {
	Sensitivity  float64 `json:"sensitivity"`  // 0=пофиг, 1=помнит каждое слово
	Sexuality    float64 `json:"sexuality"`    // 0=недотрога, 1=инициативная
	Openness     float64 `json:"openness"`     // 0=говорит прямо, 1=скрытная
	Attachment   float64 `json:"attachment"`   // 0=independent, 1=clingy
	Boldness     float64 `json:"boldness"`     // 0=мягкая, 1=дерзкая
	Humor        float64 `json:"humor"`        // 0=серьёзная, 1=клоун
	Jealousy     float64 `json:"jealousy"`     // 0=спокойная, 1=бешеная
	Romanticism  float64 `json:"romanticism"`  // 0=прагматик, 1=мечтательница
	Stubbornness float64 `json:"stubbornness"` // 0=гибкая, 1="я сказала нет"
	Empathy      float64 `json:"empathy"`      // 0=холодная, 1=чувствует всё
}

// Generate создаёт рандомную матрицу. Как у людей — выпало и выпало.
func Generate() Matrix {
	return Matrix{
		Sensitivity:  rand.Float64(),
		Sexuality:    rand.Float64(),
		Openness:     rand.Float64(),
		Attachment:   rand.Float64(),
		Boldness:     rand.Float64(),
		Humor:        rand.Float64(),
		Jealousy:     rand.Float64(),
		Romanticism:  rand.Float64(),
		Stubbornness: rand.Float64(),
		Empathy:      rand.Float64(),
	}
}

// DecayMultiplier — чем обидчивее (sensitivity), тем медленнее decay.
// Возвращает множитель для emotional decay factor.
// sensitivity=0.1 → 1.5 (быстрый decay)
// sensitivity=0.9 → 0.5 (медленный decay)
func (m Matrix) DecayMultiplier() float64 {
	return 1.5 - m.Sensitivity
}

// EvolutionRate — скорость эволюции характера. Упрямые меняются медленнее.
func (m Matrix) EvolutionRate() float64 {
	return 1.0 - m.Stubbornness*0.7
}

// JSON для инжекта в контекст.
func (m Matrix) JSON() string {
	b, _ := json.Marshal(m)
	return string(b)
}

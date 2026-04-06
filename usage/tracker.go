package usage

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Call — одно обращение к API.
type Call struct {
	Service       string    `json:"service"`        // "openrouter", "gemini_embed", "gemini_rerank", "gemini_analyzer"
	Model         string    `json:"model,omitempty"`
	PromptTokens  int       `json:"prompt_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	TotalTokens   int       `json:"total_tokens"`
	DurationMs    int64     `json:"duration_ms"`
	Timestamp     time.Time `json:"timestamp"`
}

// RequestUsage — суммарный usage за один /chat запрос.
type RequestUsage struct {
	SessionID    string  `json:"session_id"`
	Calls        []Call  `json:"calls"`
	TotalPrompt  int     `json:"total_prompt"`
	TotalOutput  int     `json:"total_output"`
	TotalTokens  int     `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	DurationMs   int64   `json:"duration_ms"`
	Timestamp    time.Time `json:"timestamp"`
}

// Stats — агрегированная статистика.
type Stats struct {
	TotalRequests  int            `json:"total_requests"`
	TotalTokens    int            `json:"total_tokens"`
	TotalCostUSD   float64        `json:"total_cost_usd"`
	ByService      map[string]ServiceStats `json:"by_service"`
	Last24h        PeriodStats    `json:"last_24h"`
	AllTime        PeriodStats    `json:"all_time"`
}

type ServiceStats struct {
	Calls        int     `json:"calls"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type PeriodStats struct {
	Requests    int     `json:"requests"`
	Tokens      int     `json:"tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// Pricing — стоимость за 1M токенов по сервисам.
var Pricing = map[string]struct{ Input, Output float64 }{
	"openrouter":       {3.0, 15.0},    // Claude Sonnet 4 через OpenRouter
	"gemini_embed":     {0.0, 0.0},     // бесплатно (free tier)
	"gemini_rerank":    {0.075, 0.30},   // Gemini Flash Lite
	"gemini_analyzer":  {0.075, 0.30},   // Gemini Flash Lite
}

// Tracker — отслеживает все API вызовы.
type Tracker struct {
	mu       sync.Mutex
	current  *RequestUsage // текущий запрос
	history  []RequestUsage
	maxHistory int
}

func NewTracker() *Tracker {
	return &Tracker{
		maxHistory: 10000,
	}
}

// StartRequest начинает отслеживание нового /chat запроса.
func (t *Tracker) StartRequest(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = &RequestUsage{
		SessionID: sessionID,
		Timestamp: time.Now(),
	}
}

// RecordCall записывает один API вызов.
func (t *Tracker) RecordCall(call Call) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return
	}
	call.Timestamp = time.Now()
	t.current.Calls = append(t.current.Calls, call)
}

// EndRequest завершает отслеживание запроса, считает итоги.
func (t *Tracker) EndRequest() *RequestUsage {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == nil {
		return nil
	}

	req := t.current
	req.DurationMs = time.Since(req.Timestamp).Milliseconds()

	for _, c := range req.Calls {
		req.TotalPrompt += c.PromptTokens
		req.TotalOutput += c.OutputTokens
		req.TotalTokens += c.TotalTokens
		req.TotalCostUSD += calcCost(c)
	}

	t.history = append(t.history, *req)
	if len(t.history) > t.maxHistory {
		t.history = t.history[len(t.history)-t.maxHistory:]
	}

	t.current = nil
	return req
}

// GetStats возвращает агрегированную статистику.
func (t *Tracker) GetStats() Stats {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := Stats{
		ByService: make(map[string]ServiceStats),
	}

	now := time.Now()
	day := now.Add(-24 * time.Hour)

	for _, req := range t.history {
		stats.TotalRequests++
		stats.TotalTokens += req.TotalTokens
		stats.TotalCostUSD += req.TotalCostUSD
		stats.AllTime.Requests++
		stats.AllTime.Tokens += req.TotalTokens
		stats.AllTime.CostUSD += req.TotalCostUSD

		if req.Timestamp.After(day) {
			stats.Last24h.Requests++
			stats.Last24h.Tokens += req.TotalTokens
			stats.Last24h.CostUSD += req.TotalCostUSD
		}

		for _, c := range req.Calls {
			s := stats.ByService[c.Service]
			s.Calls++
			s.TotalTokens += c.TotalTokens
			s.CostUSD += calcCost(c)
			stats.ByService[c.Service] = s
		}
	}

	return stats
}

// RecentJSON возвращает последние N запросов как JSON.
func (t *Tracker) RecentJSON(n int) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	start := len(t.history) - n
	if start < 0 {
		start = 0
	}
	recent := t.history[start:]
	b, _ := json.MarshalIndent(recent, "", "  ")
	return string(b)
}

func calcCost(c Call) float64 {
	price, ok := Pricing[c.Service]
	if !ok {
		return 0
	}
	return (float64(c.PromptTokens)*price.Input + float64(c.OutputTokens)*price.Output) / 1_000_000
}

func (r *RequestUsage) Summary() string {
	return fmt.Sprintf("tokens: %d (prompt: %d, output: %d), cost: $%.6f, calls: %d, duration: %dms",
		r.TotalTokens, r.TotalPrompt, r.TotalOutput, r.TotalCostUSD, len(r.Calls), r.DurationMs)
}

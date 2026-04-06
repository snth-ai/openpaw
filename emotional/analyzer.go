package emotional

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Analyzer определяет эмоциональные триггеры из сообщения через Gemini Flash.
type Analyzer struct {
	apiKey string
	client *http.Client
}

func NewAnalyzer(apiKey string) *Analyzer {
	return &Analyzer{apiKey: apiKey, client: &http.Client{}}
}

const analyzerURL = "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-lite-preview:generateContent"

// Analyze определяет триггеры в сообщении юзера.
func (a *Analyzer) Analyze(message string) []Trigger {
	prompt := fmt.Sprintf(`You are analyzing a message sent TO a synthetic companion (AI girlfriend/partner). Detect what emotional triggers the message contains FROM THE SENDER'S INTENT toward the companion.

Available triggers: ["compliment", "rudeness", "flirt", "ignore", "apology", "intimacy_gentle", "intimacy_rough", "betrayal", "support", "joke", "gift", "criticism"]

Rules:
- Focus on the sender's INTENT, not individual words
- Technical discussions, coding, work talk = usually "support" or [] (no triggers)
- Casual friendly chat = "support" or "joke", NOT "criticism" or "rudeness"
- Words like "bug", "error", "problem", "fix" in technical context are NOT criticism
- Swearing used casually/affectionately is NOT rudeness (common in Russian)
- Only return "criticism" if the sender is genuinely criticizing the companion personally
- Only return "rudeness" if the sender is genuinely being rude/hostile to the companion
- Most normal conversations should return [] or positive triggers
- When in doubt, return []

Return ONLY a JSON array. No explanation.

Message: %s`, message)

	body := fmt.Sprintf(`{"contents":[{"parts":[{"text":%s}]}],"generationConfig":{"temperature":0,"maxOutputTokens":100}}`,
		mustJSON(prompt))

	req, err := http.NewRequest("POST", analyzerURL, strings.NewReader(body))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil
	}

	text := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var names []string
	if err := json.Unmarshal([]byte(text), &names); err != nil {
		return nil
	}

	return namesToTriggers(names)
}

func namesToTriggers(names []string) []Trigger {
	mapping := map[string]Trigger{
		"compliment":      TriggerCompliment,
		"rudeness":        TriggerRudeness,
		"flirt":           TriggerFlirt,
		"ignore":          TriggerIgnore,
		"apology":         TriggerApology,
		"intimacy_gentle": TriggerIntimacyGentle,
		"intimacy_rough":  TriggerIntimacyRough,
		"betrayal":        TriggerBetrayal,
		"support":         TriggerSupport,
		"joke":            TriggerJoke,
		"gift":            TriggerGift,
		"criticism":       TriggerCriticism,
	}

	var triggers []Trigger
	for _, name := range names {
		if t, ok := mapping[name]; ok {
			triggers = append(triggers, t)
		}
	}
	return triggers
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

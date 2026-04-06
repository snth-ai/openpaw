package compact

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/openpaw/server/llm"
	"github.com/openpaw/server/memory"
)

const (
	// MaxChars — порог для compact (грубая оценка токенов: ~4 символа = 1 токен).
	// 80K символов ≈ 20K токенов.
	MaxChars = 80000

	// KeepRecent — сколько последних сообщений оставлять без сжатия.
	KeepRecent = 20
)

// Compactor сжимает историю сообщений, сохраняя факты в долгосрочную память.
type Compactor struct {
	provider llm.Provider   // для вызова LLM (извлечение + саммари)
	store    memory.Store   // для сохранения извлечённых фактов
	embedder *memory.Embedder
}

func New(provider llm.Provider, store memory.Store, embedder *memory.Embedder) *Compactor {
	return &Compactor{provider: provider, store: store, embedder: embedder}
}

// NeedsCompact проверяет нужно ли сжатие.
func NeedsCompact(messages []llm.Message) bool {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	return total > MaxChars
}

// Compact выполняет полный flow:
// 1. Выделить старые сообщения
// 2. Извлечь ключевые факты → memory_store
// 3. Сжать в саммари → заменить старые сообщения
// Возвращает новый массив messages (system + summary + recent).
func (c *Compactor) Compact(messages []llm.Message) ([]llm.Message, error) {
	if len(messages) <= KeepRecent+1 {
		return messages, nil
	}

	// Отделяем system prompt
	var systemMsg llm.Message
	rest := messages
	if len(messages) > 0 && messages[0].Role == "system" {
		systemMsg = messages[0]
		rest = messages[1:]
	}

	// Разделяем на старые и свежие
	splitAt := len(rest) - KeepRecent
	if splitAt <= 0 {
		return messages, nil
	}
	old := rest[:splitAt]
	recent := rest[splitAt:]

	// Шаг 1: извлечь факты из старых сообщений
	if err := c.extractFacts(old); err != nil {
		log.Printf("compact: extract facts error (continuing): %v", err)
		// Не фатально — продолжаем сжатие
	}

	// Шаг 2: сжать старые сообщения в саммари
	summary, err := c.summarize(old)
	if err != nil {
		return messages, fmt.Errorf("compact summarize: %w", err)
	}

	// Собираем обратно: system + summary + recent
	var result []llm.Message
	if systemMsg.Role != "" {
		result = append(result, systemMsg)
	}
	result = append(result, llm.Message{
		Role:    "system",
		Content: fmt.Sprintf("<conversation-summary>\n%s\n</conversation-summary>", summary),
	})
	result = append(result, recent...)

	log.Printf("compact: %d messages → %d (extracted facts, summarized %d old messages)",
		len(messages), len(result), len(old))

	return result, nil
}

// extractFacts просит LLM извлечь ключевые факты и сохраняет в memory.
func (c *Compactor) extractFacts(messages []llm.Message) error {
	if c.store == nil || c.embedder == nil {
		return nil
	}

	conversation := formatMessages(messages)

	extractPrompt := []llm.Message{
		{Role: "system", Content: `Extract key facts from this conversation that should be remembered long-term.
Return a JSON array of objects, each with:
- "text": the fact to remember
- "category": one of "preference", "fact", "decision", "entity", "reflection"
- "importance": 0.0-1.0

Focus on: user preferences, personal facts, decisions made, people/places mentioned, and important conclusions.
Skip: small talk, greetings, obvious context.
Return ONLY valid JSON, no markdown fences.`},
		{Role: "user", Content: conversation},
	}

	resp, err := c.provider.ChatStream(extractPrompt, nil, nil)
	if err != nil {
		return fmt.Errorf("extract call: %w", err)
	}

	// Парсим JSON из ответа
	var facts []struct {
		Text       string  `json:"text"`
		Category   string  `json:"category"`
		Importance float64 `json:"importance"`
	}

	text := strings.TrimSpace(resp.Content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	if err := json.Unmarshal([]byte(text), &facts); err != nil {
		return fmt.Errorf("parse facts: %w (response: %s)", err, truncate(resp.Content, 200))
	}

	saved := 0
	for _, f := range facts {
		if f.Text == "" || f.Importance < 0.3 {
			continue
		}

		embedding, err := c.embedder.EmbedForStorage(f.Text)
		if err != nil {
			log.Printf("compact: embed fact error: %v", err)
			continue
		}

		m := &memory.Memory{
			Text:        f.Text,
			Category:    memory.Category(f.Category),
			ContentType: memory.ContentText,
			Importance:  f.Importance,
			Embedding:   embedding,
		}

		if err := c.store.Add(m); err != nil {
			log.Printf("compact: store fact error: %v", err)
			continue
		}
		saved++
	}

	if saved > 0 {
		log.Printf("compact: extracted and saved %d facts to memory", saved)
	}

	return nil
}

// summarize просит LLM сжать сообщения в короткое саммари.
func (c *Compactor) summarize(messages []llm.Message) (string, error) {
	conversation := formatMessages(messages)

	summarizePrompt := []llm.Message{
		{Role: "system", Content: `Summarize this conversation concisely. Preserve:
- Key topics discussed
- Emotional tone and relationship dynamics
- Any unresolved questions or ongoing threads
- Important context for continuing the conversation

Write in the same language as the conversation. Be brief but don't lose important nuance.`},
		{Role: "user", Content: conversation},
	}

	resp, err := c.provider.ChatStream(summarizePrompt, nil, nil)
	if err != nil {
		return "", fmt.Errorf("summarize call: %w", err)
	}

	return resp.Content, nil
}

// GetOldMessages возвращает старые сообщения которые будут сжаты (всё кроме system + last N).
func GetOldMessages(messages []llm.Message) []llm.Message {
	rest := messages
	if len(messages) > 0 && messages[0].Role == "system" {
		rest = messages[1:]
	}
	splitAt := len(rest) - KeepRecent
	if splitAt <= 0 {
		return nil
	}
	return rest[:splitAt]
}

// FormatMessages форматирует сообщения в читаемый текст.
func FormatMessages(messages []llm.Message) string {
	return formatMessages(messages)
}

func formatMessages(messages []llm.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		if m.Role == "tool" || m.Content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

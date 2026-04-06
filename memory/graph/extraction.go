package graph

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openpaw/server/llm"
)

const extractionPrompt = `Ты — система памяти AI-компаньона. Проанализируй диалог и извлеки структурированную информацию.

Верни ТОЛЬКО валидный JSON (без markdown fences) со следующей структурой:

{
  "entities": [
    {
      "name": "каноническое имя",
      "type": "person|project|place|organization|concept|event|skill|tool",
      "aliases": ["другие варианты имени"],
      "summary": "краткое описание (1 предложение)"
    }
  ],
  "triplets": [
    {
      "subject": "имя субъекта",
      "subject_type": "тип",
      "predicate": "конкретный глагол связи (develops, lives_in, likes, frustrated_with...)",
      "object": "имя объекта",
      "object_type": "тип",
      "relation_group": "social|spatial|temporal|causal|preference|professional|emotional",
      "strength": 0.8,
      "min_strength": 0.0,
      "valid_from": "2026-03 или пусто если неизвестно",
      "context": "краткий контекст из диалога (1 предложение)"
    }
  ],
  "invalidations": [
    {
      "old_fact": "описание устаревшего факта",
      "new_fact": "новый факт который заменяет старый",
      "reason": "почему старый факт больше не верен"
    }
  ],
  "reflections": [
    {
      "text": "мысль, наблюдение или решение",
      "category": "reflection|decision|note",
      "importance": 0.7
    }
  ]
}

Правила:
- entities: ВСЕ упомянутые сущности (люди, проекты, места, инструменты)
- triplets: predicate КОНКРЕТНЫЙ ("develops", "lives_in"), НЕ generic ("related_to")
- invalidations: ТОЛЬКО если новая информация ЯВНО ПРОТИВОРЕЧИТ старой
- reflections: внутренние наблюдения, НЕ факты о мире
- strength: 0.9+ для явных фактов, 0.6-0.8 для предположений
- min_strength: минимальный порог, ниже которого факт НЕ забывается при decay
  Примеры калибровки:
  • "ест суши" → 0.0 (обычное предпочтение, может забыться)
  • "работает программистом" → 0.3 (профессия, важно но может измениться)
  • "развёлся в 2024" → 0.6 (ключевое событие)
  • "жена Даша" → 0.8 (ключевые отношения)
  • "потерял ребёнка" → 0.9 (травма, никогда не забывать)
- Не извлекай приветствия, мелкие реплики, очевидный контекст
- Если нечего извлечь — верни пустые массивы

Диалог:
%s`

// Extract вызывает LLM для извлечения графовых данных из диалога.
func Extract(provider llm.Provider, dialogue string) (*ExtractionResult, error) {
	prompt := fmt.Sprintf(extractionPrompt, dialogue)

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := provider.ChatStream(messages, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("extraction LLM call: %w", err)
	}

	text := strings.TrimSpace(resp.Content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result ExtractionResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, fmt.Errorf("parse extraction result: %w (response: %.200s)", err, text)
	}

	return &result, nil
}

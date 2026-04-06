package graph

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/openpaw/server/llm"
	"github.com/openpaw/server/memory"
	"github.com/openpaw/server/storage"
)

// DailyDigest — ежедневный сбор знаний в граф из сообщений дня.
// Запускается daemon'ом раз в сутки.
type DailyDigest struct {
	sessStore *storage.SessionStore
	pipeline  *Pipeline
	provider  llm.Provider
	memStore  memory.Store
	embedder  *memory.Embedder
}

func NewDailyDigest(sessStore *storage.SessionStore, pipeline *Pipeline, provider llm.Provider, memStore memory.Store, embedder *memory.Embedder) *DailyDigest {
	return &DailyDigest{
		sessStore: sessStore,
		pipeline:  pipeline,
		provider:  provider,
		memStore:  memStore,
		embedder:  embedder,
	}
}

// Run собирает все сообщения за сегодня и прогоняет через graph extraction.
func (d *DailyDigest) Run() {
	log.Println("daily digest: starting")

	// Получаем все сессии
	sessionIDs, err := d.sessStore.List()
	if err != nil {
		log.Printf("daily digest: list sessions error: %v", err)
		return
	}

	totalMessages := 0
	var allDialogues []string

	for _, sessionID := range sessionIDs {
		messages, err := d.sessStore.Load(sessionID)
		if err != nil || len(messages) == 0 {
			continue
		}

		// Фильтруем только сегодняшние сообщения
		// (у нас нет timestamp в messages, берём последние N как proxy)
		// TODO: добавить timestamp в messages для точной фильтрации
		// Пока берём все — digest обработает дубли через entity resolution
		todayMessages := messages
		if len(todayMessages) > 50 {
			todayMessages = todayMessages[len(todayMessages)-50:]
		}

		// Форматируем в диалог
		var sb strings.Builder
		for _, msg := range todayMessages {
			if msg.Role == "system" || msg.Role == "tool" || msg.Content == "" {
				continue
			}
			// Убираем [context] блоки
			content := msg.Content
			if idx := strings.Index(content, "\n\n[context]"); idx > 0 {
				content = content[:idx]
			}
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, content))
		}

		dialogue := sb.String()
		if len(dialogue) > 100 { // skip пустые/тривиальные
			allDialogues = append(allDialogues, dialogue)
			totalMessages += len(todayMessages)
		}
	}

	if len(allDialogues) == 0 {
		log.Println("daily digest: no messages to process")
		return
	}

	// Объединяем все диалоги (с лимитом чтобы не взорвать контекст)
	combined := strings.Join(allDialogues, "\n---\n")
	if len(combined) > 50000 { // ~12K tokens max
		combined = combined[:50000]
	}

	log.Printf("daily digest: processing %d messages from %d sessions", totalMessages, len(allDialogues))

	// Graph extraction
	err = d.pipeline.ProcessDialogue(combined, time.Now())
	if err != nil {
		log.Printf("daily digest: graph extraction error: %v", err)
		return
	}

	// Daily diary entry — синт пишет summary дня в Memory Log
	d.writeDiary(combined)

	log.Println("daily digest: done")
}

const diaryPrompt = `Ты — AI-компаньон. Напиши краткий дневник за сегодняшний день на основе диалогов ниже.

Формат: 2-5 предложений. Что обсуждали, что узнала нового, что чувствовала. Пиши от первого лица, женский род.

Если ничего значимого не было — напиши "Спокойный день, ничего особенного."

Диалоги:
%s`

func (d *DailyDigest) writeDiary(dialogue string) {
	if d.memStore == nil || d.embedder == nil {
		return
	}

	// Обрезаем для промпта
	text := dialogue
	if len(text) > 20000 {
		text = text[:20000]
	}

	prompt := fmt.Sprintf(diaryPrompt, text)
	resp, err := d.provider.ChatStream([]llm.Message{
		{Role: "user", Content: prompt},
	}, nil, nil)
	if err != nil {
		log.Printf("daily digest: diary error: %v", err)
		return
	}

	diary := strings.TrimSpace(resp.Content)
	if diary == "" {
		return
	}

	// Сохраняем в Memory Log как reflection
	embedding, err := d.embedder.EmbedForStorage(diary)
	if err != nil {
		return
	}

	today := time.Now().Format("2006-01-02")
	m := &memory.Memory{
		Text:        fmt.Sprintf("[diary %s] %s", today, diary),
		Category:    memory.CategoryReflection,
		ContentType: memory.ContentText,
		Importance:  0.6,
		Embedding:   embedding,
		CreatedAt:   time.Now(),
	}
	d.memStore.Add(m)

	log.Printf("daily digest: diary saved (%d chars)", len(diary))
}

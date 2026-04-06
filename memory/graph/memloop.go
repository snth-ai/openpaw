package graph

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/openpaw/server/llm"
	"github.com/openpaw/server/memory"
)

// MemoryLoop — отдельный memory loop на дешёвой модели.
// Только retrieve. Store происходит при compact или через memory_store тул.
type MemoryLoop struct {
	store     GraphStore
	provider  llm.Provider // дешёвая модель для retrieve decisions
	embedder  *memory.Embedder
	retriever *Retriever
}

func NewMemoryLoop(store GraphStore, cheapProvider llm.Provider, embedder *memory.Embedder) *MemoryLoop {
	return &MemoryLoop{
		store:     store,
		provider:  cheapProvider,
		embedder:  embedder,
		retriever: NewRetriever(store, embedder),
	}
}

// MemoryLoopResult — результат memory loop для инжекта в agent loop.
type MemoryLoopResult struct {
	Knowledge string // <knowledge> блок для prompt
	TokensEst int    // примерная оценка токенов
}

const memoryLoopPrompt = `Какие из этих сущностей релевантны сообщению пользователя? Верни ТОЛЬКО JSON массив имён. Если ничего не релевантно — пустой массив []. Максимум 5.

Сущности:
%s

Сообщение: "%s"

Ответ (только JSON массив строк):`

type memLoopResponse struct {
	RelevantEntities []string `json:"relevant_entities"`
}

// Run выполняет memory loop: ТОЛЬКО retrieve.
// Store происходит при compact (batch, качественно через Claude) или через memory_store тул.
func (ml *MemoryLoop) Run(userMessage string) MemoryLoopResult {
	// Pre-filter: entity detection без API call
	detectedNodes := ml.retriever.detectEntities(userMessage)
	if len(detectedNodes) == 0 {
		log.Printf("memory loop: no entities detected, skipping")
		return MemoryLoopResult{}
	}

	log.Printf("memory loop: detected %d entities, running retrieve", len(detectedNodes))

	// Получаем список всех нод для промпта
	nodes, err := ml.store.GetAllNodes()
	if err != nil || len(nodes) == 0 {
		knowledge := ml.retriever.RetrieveContext(userMessage)
		return MemoryLoopResult{Knowledge: knowledge, TokensEst: len(knowledge) / 4}
	}

	// Формируем компактный список нод
	var nodeList strings.Builder
	for _, n := range nodes {
		nodeList.WriteString(fmt.Sprintf("- %s (%s)", n.Name, n.Type))
		if len(n.Aliases) > 0 {
			nodeList.WriteString(fmt.Sprintf(" [aka: %s]", strings.Join(n.Aliases, ", ")))
		}
		nodeList.WriteString("\n")
	}

	// LLM call на дешёвой модели
	prompt := fmt.Sprintf(memoryLoopPrompt, nodeList.String(), userMessage)
	resp, err := ml.provider.ChatStream([]llm.Message{
		{Role: "user", Content: prompt},
	}, nil, nil)
	if err != nil {
		log.Printf("memory loop: LLM error: %v, falling back to entity detection", err)
		knowledge := ml.retriever.RetrieveContext(userMessage)
		return MemoryLoopResult{Knowledge: knowledge, TokensEst: len(knowledge) / 4}
	}

	// Парсим ответ
	text := strings.TrimSpace(resp.Content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	// Парсим как массив строк или как объект с relevant_entities
	var result memLoopResponse
	var names []string
	if err := json.Unmarshal([]byte(text), &names); err == nil {
		result.RelevantEntities = names
	} else if err := json.Unmarshal([]byte(text), &result); err != nil {
		log.Printf("memory loop: parse error: %v, falling back to entity detection", err)
		knowledge := ml.retriever.RetrieveContext(userMessage)
		return MemoryLoopResult{Knowledge: knowledge, TokensEst: len(knowledge) / 4}
	}

	// === RETRIEVE: подтянуть релевантные сущности ===
	var knowledge string
	if len(result.RelevantEntities) > 0 {
		// Найти ноды по именам
		foundNodes, _ := ml.store.FindNodesByNames(result.RelevantEntities)
		if len(foundNodes) > 0 {
			var nodeIDs []string
			for _, n := range foundNodes {
				nodeIDs = append(nodeIDs, n.ID)
			}
			sg, err := ml.store.GetSubgraph(nodeIDs, false)
			if err == nil && len(sg.Edges) > 0 {
				// Лимит: top-15 edges by strength
				if len(sg.Edges) > 15 {
					// Сортируем по strength
					for i := 1; i < len(sg.Edges); i++ {
						for j := i; j > 0 && sg.Edges[j].Strength > sg.Edges[j-1].Strength; j-- {
							sg.Edges[j], sg.Edges[j-1] = sg.Edges[j-1], sg.Edges[j]
						}
					}
					sg.Edges = sg.Edges[:15]
				}
				knowledge = RenderSubgraph(sg)
			}
		}
	}

	log.Printf("memory loop: relevant=%d entities, knowledge=%d bytes",
		len(result.RelevantEntities), len(knowledge))

	// Store НЕ здесь. Граф наполняется при compact (batch, через Claude)
	// или через memory_store тул (синт сама решает).

	return MemoryLoopResult{
		Knowledge: knowledge,
		TokensEst: len(knowledge) / 4,
	}
}

// Store не здесь. Граф наполняется:
// 1. При compact — batch extraction через Claude (видит 20-50 сообщений, качественно)
// 2. Через memory_store тул — синт сама решает что важно запомнить

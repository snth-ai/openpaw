package llm

// Provider — абстракция LLM провайдера.
// OpenRouter, прямой Anthropic, Gemini — всё реализует этот интерфейс.
type Provider interface {
	ChatStream(messages []Message, tools []map[string]any, onChunk func(string)) (*Response, error)
}

package llm

import "encoding/json"

// Message — OpenAI-совместимый формат сообщения.
type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	ContentParts []ContentPart `json:"-"` // для block-style с cache_control (сериализуется через MarshalJSON)
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
}

// ContentPart — блок контента с опциональным cache_control.
type ContentPart struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *CacheControlPart `json:"cache_control,omitempty"`
}

type CacheControlPart struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

// MarshalJSON — кастомная сериализация: если ContentParts заполнен, используем block format.
func (m Message) MarshalJSON() ([]byte, error) {
	type rawMessage struct {
		Role         string        `json:"role"`
		Content      any           `json:"content,omitempty"`
		ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
		ToolCallID   string        `json:"tool_call_id,omitempty"`
	}

	raw := rawMessage{
		Role:       m.Role,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
	}

	if len(m.ContentParts) > 0 {
		raw.Content = m.ContentParts
	} else if m.Content != "" {
		raw.Content = m.Content
	}

	return json.Marshal(raw)
}




// UnmarshalJSON — content может быть строкой или массивом блоков.
func (m *Message) UnmarshalJSON(data []byte) error {
	type rawMessage struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content,omitempty"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
	}

	var raw rawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.Role = raw.Role
	m.ToolCalls = raw.ToolCalls
	m.ToolCallID = raw.ToolCallID

	if len(raw.Content) > 0 {
		var s string
		if err := json.Unmarshal(raw.Content, &s); err == nil {
			m.Content = s
			return nil
		}
		var parts []ContentPart
		if err := json.Unmarshal(raw.Content, &parts); err == nil {
			m.ContentParts = parts
			for _, p := range parts {
				m.Content += p.Text
			}
			return nil
		}
	}

	return nil
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage — информация о потреблении токенов.
type Usage struct {
	PromptTokens          int                    `json:"prompt_tokens"`
	OutputTokens          int                    `json:"completion_tokens"`
	TotalTokens           int                    `json:"total_tokens"`
	PromptTokensDetails   *PromptTokensDetails   `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type PromptTokensDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// Response — результат вызова LLM.
type Response struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
}

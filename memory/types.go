package memory

import "time"

// Category — тип воспоминания.
type Category string

const (
	CategoryPreference Category = "preference" // предпочтения юзера
	CategoryFact       Category = "fact"       // факты, данные
	CategoryDecision   Category = "decision"   // решения, договорённости
	CategoryEntity     Category = "entity"     // люди, места, проекты
	CategoryReflection Category = "reflection" // мысли/выводы синта
)

// ContentType — тип контента (для мультимодальных эмбеддингов).
type ContentType string

const (
	ContentText  ContentType = "text"
	ContentImage ContentType = "image"
	ContentAudio ContentType = "audio"
	ContentVideo ContentType = "video"
)

// Memory — единица памяти синта.
type Memory struct {
	ID          string      `json:"id"`
	Text        string      `json:"text"`
	Category    Category    `json:"category"`
	ContentType ContentType `json:"content_type"`
	Scope       string      `json:"scope"` // "default" или именованный скоуп
	Importance  float64     `json:"importance"`
	Embedding   []float32   `json:"embedding,omitempty"`
	AccessCount int         `json:"access_count"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	LastAccess  time.Time   `json:"last_access"`
}

// SearchResult — результат поиска с оценкой релевантности.
type SearchResult struct {
	Memory
	Distance    float32 `json:"distance"`      // от vector search
	RerankerScore float64 `json:"reranker_score"` // от Gemini Flash reranker
}

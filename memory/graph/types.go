package graph

import "time"

// Node — сущность в графе знаний.
type Node struct {
	ID           string         `json:"id" db:"id"`
	Name         string         `json:"name" db:"name"`
	Type         string         `json:"type" db:"type"`
	Aliases      []string       `json:"aliases"`
	Tags         []string       `json:"tags"`
	Summary      string         `json:"summary" db:"summary"`
	Properties   map[string]any `json:"properties"`
	Embedding    []float32      `json:"-"`
	MentionCount int            `json:"mention_count" db:"mention_count"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
}

// Edge — связь между сущностями. Bi-temporal model.
type Edge struct {
	ID            string     `json:"id" db:"id"`
	FromID        string     `json:"from_id" db:"from_id"`
	ToID          string     `json:"to_id" db:"to_id"`
	Relation      string     `json:"relation" db:"relation"`
	RelationGroup string     `json:"relation_group" db:"relation_group"`
	Strength      float64    `json:"strength" db:"strength"`
	MinStrength   float64    `json:"min_strength" db:"min_strength"` // floor — edge не decay'ится ниже этого
	ValidFrom     *time.Time `json:"valid_from" db:"valid_from"`
	ValidUntil    *time.Time `json:"valid_until" db:"valid_until"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	InvalidatedAt *time.Time `json:"invalidated_at" db:"invalidated_at"`
	InvalidatedBy string     `json:"invalidated_by" db:"invalidated_by"`
	Context       string     `json:"context" db:"context"`
	SourceEpisode string     `json:"source_episode" db:"source_episode"`
	Properties    map[string]any `json:"properties"`
}

// IsActive возвращает true если edge не инвалидирован.
func (e Edge) IsActive() bool {
	return e.InvalidatedAt == nil
}

// Episode — запись о событии/диалоге.
type Episode struct {
	ID        string    `json:"id" db:"id"`
	Summary   string    `json:"summary" db:"summary"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	NodeIDs   []string  `json:"node_ids"`
	EdgeIDs   []string  `json:"edge_ids"`
}

// Subgraph — подграф для retrieval.
type Subgraph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// GraphStats — статистика графа.
type GraphStats struct {
	TotalNodes       int `json:"total_nodes"`
	TotalEdges       int `json:"total_edges"`
	ActiveEdges      int `json:"active_edges"`
	InvalidatedEdges int `json:"invalidated_edges"`
	TotalEpisodes    int `json:"total_episodes"`
}

// ExtractionResult — результат extraction из LLM.
type ExtractionResult struct {
	Entities      []ExtractedEntity      `json:"entities"`
	Triplets      []ExtractedTriplet     `json:"triplets"`
	Invalidations []ExtractedInvalidation `json:"invalidations"`
	Reflections   []ExtractedReflection  `json:"reflections"`
}

type ExtractedEntity struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Aliases []string `json:"aliases"`
	Summary string   `json:"summary"`
}

type ExtractedTriplet struct {
	Subject       string  `json:"subject"`
	SubjectType   string  `json:"subject_type"`
	Predicate     string  `json:"predicate"`
	Object        string  `json:"object"`
	ObjectType    string  `json:"object_type"`
	RelationGroup string  `json:"relation_group"`
	Strength      float64 `json:"strength"`
	MinStrength   float64 `json:"min_strength"` // 0=обычный, 0.3+=важный, 0.8+=никогда не забывать
	ValidFrom     string  `json:"valid_from"`
	Context       string  `json:"context"`
}

type ExtractedInvalidation struct {
	OldFact string `json:"old_fact"`
	NewFact string `json:"new_fact"`
	Reason  string `json:"reason"`
}

type ExtractedReflection struct {
	Text       string  `json:"text"`
	Category   string  `json:"category"`
	Importance float64 `json:"importance"`
}

package graphtypes

import "time"

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

type Edge struct {
	ID            string         `json:"id" db:"id"`
	FromID        string         `json:"from_id" db:"from_id"`
	ToID          string         `json:"to_id" db:"to_id"`
	Relation      string         `json:"relation" db:"relation"`
	RelationGroup string         `json:"relation_group" db:"relation_group"`
	Strength      float64        `json:"strength" db:"strength"`
	MinStrength   float64        `json:"min_strength" db:"min_strength"`
	ValidFrom     *time.Time     `json:"valid_from" db:"valid_from"`
	ValidUntil    *time.Time     `json:"valid_until" db:"valid_until"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	InvalidatedAt *time.Time     `json:"invalidated_at" db:"invalidated_at"`
	InvalidatedBy string         `json:"invalidated_by" db:"invalidated_by"`
	Context       string         `json:"context" db:"context"`
	SourceEpisode string         `json:"source_episode" db:"source_episode"`
	Properties    map[string]any `json:"properties"`
}

func (e Edge) IsActive() bool {
	return e.InvalidatedAt == nil
}

type Episode struct {
	ID        string    `json:"id" db:"id"`
	Summary   string    `json:"summary" db:"summary"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	NodeIDs   []string  `json:"node_ids"`
	EdgeIDs   []string  `json:"edge_ids"`
}

type Subgraph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type GraphStats struct {
	TotalNodes       int `json:"total_nodes"`
	TotalEdges       int `json:"total_edges"`
	ActiveEdges      int `json:"active_edges"`
	InvalidatedEdges int `json:"invalidated_edges"`
	TotalEpisodes    int `json:"total_episodes"`
}

type GraphStore interface {
	GetNode(id string) (*Node, error)
	GetNodeByName(name string) (*Node, error)
	FindNodesByNames(names []string) ([]*Node, error)
	FindNodesByEmbedding(emb []float32, threshold float64, limit int) ([]*Node, error)
	UpsertNode(node *Node) error
	DeleteNode(id string) error
	IncrementMentionCount(id string) error

	AddEdge(edge *Edge) error
	UpdateEdge(edge *Edge) error
	GetActiveEdgesForNode(nodeID string) ([]Edge, error)
	GetAllEdgesForNode(nodeID string) ([]Edge, error)
	GetActiveEdgesByGroup(nodeID string, group string) ([]Edge, error)
	FindDuplicateEdge(fromID, toID, relation string) (*Edge, error)
	InvalidateEdge(id string, replacedBy string, reason string) error
	FindEdgesByContextEmbedding(emb []float32, threshold float64, limit int) ([]Edge, error)

	AddEpisode(episode *Episode) error

	GetAllNodes() ([]Node, error)
	GetAllActiveEdges() ([]Edge, error)
	GetSubgraph(nodeIDs []string, includeInvalidated bool) (*Subgraph, error)
	GetUserProfile(userNodeID string, limit int) (*Subgraph, error)

	DecayEdgeStrengths(factor float64) (int, error)
	PruneWeakInvalidatedEdges(minStrength float64) (int, error)
	PruneOrphanNodes(minMentions int) (int, error)
	GetStats() (*GraphStats, error)

	Migrate() error
}

type GraphDecayConfig struct {
	EdgeDecayFactor float64
	MinEdgeStrength float64
	MinMentionCount int
	DecayInterval   time.Duration
}

func DefaultGraphDecayConfig() GraphDecayConfig {
	return GraphDecayConfig{
		EdgeDecayFactor: 0.998,
		MinEdgeStrength: 0.1,
		MinMentionCount: 3,
		DecayInterval:   24 * time.Hour,
	}
}

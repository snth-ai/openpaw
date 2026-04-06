package graph

import "time"

// GraphStore — интерфейс хранилища графа знаний.
type GraphStore interface {
	// Nodes
	GetNode(id string) (*Node, error)
	GetNodeByName(name string) (*Node, error)
	FindNodesByNames(names []string) ([]*Node, error)
	FindNodesByEmbedding(emb []float32, threshold float64, limit int) ([]*Node, error)
	UpsertNode(node *Node) error
	DeleteNode(id string) error
	IncrementMentionCount(id string) error

	// Edges
	AddEdge(edge *Edge) error
	UpdateEdge(edge *Edge) error
	GetActiveEdgesForNode(nodeID string) ([]Edge, error)
	GetAllEdgesForNode(nodeID string) ([]Edge, error)
	GetActiveEdgesByGroup(nodeID string, group string) ([]Edge, error)
	FindDuplicateEdge(fromID, toID, relation string) (*Edge, error)
	InvalidateEdge(id string, replacedBy string, reason string) error
	FindEdgesByContextEmbedding(emb []float32, threshold float64, limit int) ([]Edge, error)

	// Episodes
	AddEpisode(episode *Episode) error

	// Retrieval
	GetAllNodes() ([]Node, error)
	GetAllActiveEdges() ([]Edge, error)
	GetSubgraph(nodeIDs []string, includeInvalidated bool) (*Subgraph, error)
	GetUserProfile(userNodeID string, limit int) (*Subgraph, error)

	// Maintenance
	DecayEdgeStrengths(factor float64) (int, error)
	PruneWeakInvalidatedEdges(minStrength float64) (int, error)
	PruneOrphanNodes(minMentions int) (int, error)
	GetStats() (*GraphStats, error)

	// Migration
	Migrate() error
}

// DecayConfig для графа.
type GraphDecayConfig struct {
	EdgeDecayFactor   float64       // 0.998 = 0.2% в день
	MinEdgeStrength   float64       // 0.1 — ниже удаляем invalidated
	MinMentionCount   int           // 3 — ниже удаляем orphan nodes
	DecayInterval     time.Duration // как часто запускать
}

func DefaultGraphDecayConfig() GraphDecayConfig {
	return GraphDecayConfig{
		EdgeDecayFactor: 0.998,
		MinEdgeStrength: 0.1,
		MinMentionCount: 3,
		DecayInterval:   24 * time.Hour,
	}
}

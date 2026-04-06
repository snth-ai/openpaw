package graphtypes

import (
	"sync"
	"time"
)

// MockGraphStore is an in-memory implementation of GraphStore for testing.
type MockGraphStore struct {
	mu       sync.RWMutex
	Nodes    map[string]*Node
	Edges    map[string]*Edge
	Episodes map[string]*Episode
}

// NewMockGraphStore creates a new empty mock store.
func NewMockGraphStore() *MockGraphStore {
	return &MockGraphStore{
		Nodes:    make(map[string]*Node),
		Edges:    make(map[string]*Edge),
		Episodes: make(map[string]*Episode),
	}
}

// Nodes
func (m *MockGraphStore) GetNode(id string) (*Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Nodes[id], nil
}

func (m *MockGraphStore) GetNodeByName(name string) (*Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, node := range m.Nodes {
		if node.Name == name {
			return node, nil
		}
	}
	return nil, nil
}

func (m *MockGraphStore) FindNodesByNames(names []string) ([]*Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Node
	for _, name := range names {
		for _, node := range m.Nodes {
			if node.Name == name {
				result = append(result, node)
				break
			}
		}
	}
	return result, nil
}

func (m *MockGraphStore) FindNodesByEmbedding(emb []float32, threshold float64, limit int) ([]*Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Simple implementation: return all nodes (no actual embedding search)
	var result []*Node
	count := 0
	for _, node := range m.Nodes {
		result = append(result, node)
		count++
		if limit > 0 && count >= limit {
			break
		}
	}
	return result, nil
}

func (m *MockGraphStore) UpsertNode(node *Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Nodes[node.ID] = node
	return nil
}

func (m *MockGraphStore) DeleteNode(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Nodes, id)
	return nil
}

func (m *MockGraphStore) IncrementMentionCount(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if node, ok := m.Nodes[id]; ok {
		node.MentionCount++
	}
	return nil
}

// Edges
func (m *MockGraphStore) AddEdge(edge *Edge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Edges[edge.ID] = edge
	return nil
}

func (m *MockGraphStore) UpdateEdge(edge *Edge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Edges[edge.ID] = edge
	return nil
}

func (m *MockGraphStore) GetActiveEdgesForNode(nodeID string) ([]Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Edge
	for _, edge := range m.Edges {
		if (edge.FromID == nodeID || edge.ToID == nodeID) && edge.IsActive() {
			result = append(result, *edge)
		}
	}
	return result, nil
}

func (m *MockGraphStore) GetAllEdgesForNode(nodeID string) ([]Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Edge
	for _, edge := range m.Edges {
		if edge.FromID == nodeID || edge.ToID == nodeID {
			result = append(result, *edge)
		}
	}
	return result, nil
}

func (m *MockGraphStore) GetActiveEdgesByGroup(nodeID string, group string) ([]Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Edge
	for _, edge := range m.Edges {
		if edge.FromID == nodeID && edge.RelationGroup == group && edge.IsActive() {
			result = append(result, *edge)
		}
	}
	return result, nil
}

func (m *MockGraphStore) FindDuplicateEdge(fromID, toID, relation string) (*Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, edge := range m.Edges {
		if edge.FromID == fromID && edge.ToID == toID && edge.Relation == relation && edge.IsActive() {
			return edge, nil
		}
	}
	return nil, nil
}

func (m *MockGraphStore) InvalidateEdge(id string, replacedBy string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if edge, ok := m.Edges[id]; ok {
		now := time.Now()
		edge.InvalidatedAt = &now
		edge.InvalidatedBy = replacedBy
	}
	return nil
}

func (m *MockGraphStore) FindEdgesByContextEmbedding(emb []float32, threshold float64, limit int) ([]Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Edge
	count := 0
	for _, edge := range m.Edges {
		if edge.IsActive() {
			result = append(result, *edge)
			count++
			if limit > 0 && count >= limit {
				break
			}
		}
	}
	return result, nil
}

// Episodes
func (m *MockGraphStore) AddEpisode(episode *Episode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Episodes[episode.ID] = episode
	return nil
}

// Retrieval
func (m *MockGraphStore) GetAllNodes() ([]Node, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Node
	for _, node := range m.Nodes {
		result = append(result, *node)
	}
	return result, nil
}

func (m *MockGraphStore) GetAllActiveEdges() ([]Edge, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []Edge
	for _, edge := range m.Edges {
		if edge.IsActive() {
			result = append(result, *edge)
		}
	}
	return result, nil
}

func (m *MockGraphStore) GetSubgraph(nodeIDs []string, includeInvalidated bool) (*Subgraph, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodeSet := make(map[string]bool)
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}

	var nodes []Node
	for _, id := range nodeIDs {
		if node, ok := m.Nodes[id]; ok {
			nodes = append(nodes, *node)
		}
	}

	var edges []Edge
	for _, edge := range m.Edges {
		if nodeSet[edge.FromID] || nodeSet[edge.ToID] {
			if includeInvalidated || edge.IsActive() {
				edges = append(edges, *edge)
			}
		}
	}

	return &Subgraph{Nodes: nodes, Edges: edges}, nil
}

func (m *MockGraphStore) GetUserProfile(userNodeID string, limit int) (*Subgraph, error) {
	return m.GetSubgraph([]string{userNodeID}, false)
}

// Maintenance
func (m *MockGraphStore) DecayEdgeStrengths(factor float64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, edge := range m.Edges {
		if edge.IsActive() {
			edge.Strength *= factor
			if edge.Strength < edge.MinStrength {
				edge.Strength = edge.MinStrength
			}
			count++
		}
	}
	return count, nil
}

func (m *MockGraphStore) PruneWeakInvalidatedEdges(minStrength float64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for id, edge := range m.Edges {
		if !edge.IsActive() && edge.Strength < minStrength {
			delete(m.Edges, id)
			count++
		}
	}
	return count, nil
}

func (m *MockGraphStore) PruneOrphanNodes(minMentions int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for id, node := range m.Nodes {
		if node.MentionCount < minMentions {
			delete(m.Nodes, id)
			count++
		}
	}
	return count, nil
}

func (m *MockGraphStore) GetStats() (*GraphStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeEdges := 0
	invalidatedEdges := 0
	for _, edge := range m.Edges {
		if edge.IsActive() {
			activeEdges++
		} else {
			invalidatedEdges++
		}
	}

	return &GraphStats{
		TotalNodes:       len(m.Nodes),
		TotalEdges:       len(m.Edges),
		ActiveEdges:      activeEdges,
		InvalidatedEdges: invalidatedEdges,
		TotalEpisodes:    len(m.Episodes),
	}, nil
}

func (m *MockGraphStore) Migrate() error {
	return nil
}

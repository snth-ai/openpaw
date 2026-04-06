package graphtypes

import (
	"testing"
	"time"
)

func TestEdge_IsActive(t *testing.T) {
	tests := []struct {
		name string
		edge Edge
		want bool
	}{
		{
			name: "active edge",
			edge: Edge{ID: "1", FromID: "a", ToID: "b", Relation: "knows"},
			want: true,
		},
		{
			name: "invalidated edge",
			edge: Edge{
				ID:            "2",
				FromID:        "a",
				ToID:          "c",
				Relation:      "likes",
				InvalidatedAt: ptrTime(time.Now()),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.edge.IsActive()
			if got != tt.want {
				t.Errorf("Edge.IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultGraphDecayConfig(t *testing.T) {
	cfg := DefaultGraphDecayConfig()

	if cfg.EdgeDecayFactor != 0.998 {
		t.Errorf("EdgeDecayFactor = %v, want 0.998", cfg.EdgeDecayFactor)
	}
	if cfg.MinEdgeStrength != 0.1 {
		t.Errorf("MinEdgeStrength = %v, want 0.1", cfg.MinEdgeStrength)
	}
	if cfg.MinMentionCount != 3 {
		t.Errorf("MinMentionCount = %v, want 3", cfg.MinMentionCount)
	}
	if cfg.DecayInterval != 24*time.Hour {
		t.Errorf("DecayInterval = %v, want 24h", cfg.DecayInterval)
	}
}

func TestMockGraphStore_UpsertNode(t *testing.T) {
	store := NewMockGraphStore()

	node := &Node{
		ID:           "node-1",
		Name:         "Alice",
		Type:         "person",
		MentionCount: 1,
	}

	err := store.UpsertNode(node)
	if err != nil {
		t.Fatalf("UpsertNode() error = %v", err)
	}

	got, err := store.GetNode("node-1")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("got.Name = %q, want %q", got.Name, "Alice")
	}
}

func TestMockGraphStore_AddEdge(t *testing.T) {
	store := NewMockGraphStore()

	edge := &Edge{
		ID:       "edge-1",
		FromID:   "a",
		ToID:     "b",
		Relation: "knows",
		Strength: 0.8,
	}

	err := store.AddEdge(edge)
	if err != nil {
		t.Fatalf("AddEdge() error = %v", err)
	}

	edges, err := store.GetActiveEdgesForNode("a")
	if err != nil {
		t.Fatalf("GetActiveEdgesForNode() error = %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("len(edges) = %d, want 1", len(edges))
	}
	if edges[0].Relation != "knows" {
		t.Errorf("edges[0].Relation = %q, want %q", edges[0].Relation, "knows")
	}
}

func TestMockGraphStore_DecayEdgeStrengths(t *testing.T) {
	store := NewMockGraphStore()

	store.AddEdge(&Edge{ID: "1", FromID: "a", ToID: "b", Strength: 1.0, MinStrength: 0.1})
	store.AddEdge(&Edge{ID: "2", FromID: "c", ToID: "d", Strength: 0.5, MinStrength: 0.1})

	count, err := store.DecayEdgeStrengths(0.9)
	if err != nil {
		t.Fatalf("DecayEdgeStrengths() error = %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	edge1, _ := store.GetNode("1")
	_ = edge1
	edges, _ := store.GetAllActiveEdges()
	for _, e := range edges {
		if e.ID == "1" && e.Strength != 0.9 {
			t.Errorf("edge 1 Strength = %v, want 0.9", e.Strength)
		}
		if e.ID == "2" && e.Strength != 0.45 {
			t.Errorf("edge 2 Strength = %v, want 0.45", e.Strength)
		}
	}
}

func TestMockGraphStore_InvalidateEdge(t *testing.T) {
	store := NewMockGraphStore()

	store.AddEdge(&Edge{ID: "1", FromID: "a", ToID: "b", Relation: "knows"})

	err := store.InvalidateEdge("1", "edge-2", "replaced")
	if err != nil {
		t.Fatalf("InvalidateEdge() error = %v", err)
	}

	edge, _ := store.FindDuplicateEdge("a", "b", "knows")
	if edge != nil {
		t.Error("FindDuplicateEdge should return nil for invalidated edge")
	}

	edges, _ := store.GetActiveEdgesForNode("a")
	if len(edges) != 0 {
		t.Errorf("GetActiveEdgesForNode returned %d edges, want 0", len(edges))
	}
}

func TestMockGraphStore_GetStats(t *testing.T) {
	store := NewMockGraphStore()

	store.UpsertNode(&Node{ID: "1", Name: "Alice"})
	store.UpsertNode(&Node{ID: "2", Name: "Bob"})
	store.AddEdge(&Edge{ID: "1", FromID: "1", ToID: "2", Relation: "knows"})

	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}

	if stats.TotalNodes != 2 {
		t.Errorf("TotalNodes = %d, want 2", stats.TotalNodes)
	}
	if stats.TotalEdges != 1 {
		t.Errorf("TotalEdges = %d, want 1", stats.TotalEdges)
	}
	if stats.ActiveEdges != 1 {
		t.Errorf("ActiveEdges = %d, want 1", stats.ActiveEdges)
	}
}

func TestMockGraphStore_Subgraph(t *testing.T) {
	store := NewMockGraphStore()

	store.UpsertNode(&Node{ID: "1", Name: "Alice"})
	store.UpsertNode(&Node{ID: "2", Name: "Bob"})
	store.AddEdge(&Edge{ID: "1", FromID: "1", ToID: "2", Relation: "knows"})

	sg, err := store.GetSubgraph([]string{"1"}, false)
	if err != nil {
		t.Fatalf("GetSubgraph() error = %v", err)
	}

	if len(sg.Nodes) != 1 {
		t.Errorf("len(Nodes) = %d, want 1", len(sg.Nodes))
	}
	if len(sg.Edges) != 1 {
		t.Errorf("len(Edges) = %d, want 1", len(sg.Edges))
	}
}

// Helper
func ptrTime(t time.Time) *time.Time {
	return &t
}

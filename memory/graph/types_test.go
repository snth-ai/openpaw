package graph

import (
	"testing"
	"time"
)

func TestEdge_IsActive(t *testing.T) {
	tests := []struct {
		name  string
		edge  Edge
		want  bool
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

func TestNode_JSON(t *testing.T) {
	node := Node{
		ID:           "node-1",
		Name:         "Alice",
		Type:         "person",
		Aliases:      []string{"Ally", "Lice"},
		Tags:         []string{"friend"},
		Summary:      "A close friend",
		MentionCount: 5,
	}

	// Verify fields are set correctly
	if node.ID != "node-1" {
		t.Errorf("ID = %q, want %q", node.ID, "node-1")
	}
	if node.Name != "Alice" {
		t.Errorf("Name = %q, want %q", node.Name, "Alice")
	}
	if len(node.Aliases) != 2 {
		t.Errorf("Aliases length = %d, want 2", len(node.Aliases))
	}
}

func TestEdge_BiTemporal(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	edge := Edge{
		ID:          "edge-1",
		FromID:      "a",
		ToID:        "b",
		Relation:    "works_at",
		Strength:    0.8,
		MinStrength: 0.2,
		ValidFrom:   &yesterday,
		ValidUntil:  nil, // still valid
		CreatedAt:   yesterday,
	}

	// Verify bi-temporal fields
	if edge.ValidFrom == nil {
		t.Error("ValidFrom should not be nil")
	}
	if edge.ValidUntil != nil {
		t.Error("ValidUntil should be nil for active edge")
	}
	if edge.MinStrength != 0.2 {
		t.Errorf("MinStrength = %v, want 0.2", edge.MinStrength)
	}
}

func TestSubgraph(t *testing.T) {
	sg := Subgraph{
		Nodes: []Node{
			{ID: "n1", Name: "Alice"},
			{ID: "n2", Name: "Bob"},
		},
		Edges: []Edge{
			{ID: "e1", FromID: "n1", ToID: "n2", Relation: "knows"},
		},
	}

	if len(sg.Nodes) != 2 {
		t.Errorf("Nodes length = %d, want 2", len(sg.Nodes))
	}
	if len(sg.Edges) != 1 {
		t.Errorf("Edges length = %d, want 1", len(sg.Edges))
	}
}

// Helper function
func ptrTime(t time.Time) *time.Time {
	return &t
}

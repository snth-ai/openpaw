// Package graph provides knowledge graph storage and retrieval.
// Type definitions are in graphtypes package for testability.
package graph

import (
	"github.com/openpaw/server/memory/graphtypes"
)

// Type aliases for backward compatibility
type Node = graphtypes.Node
type Edge = graphtypes.Edge
type Episode = graphtypes.Episode
type Subgraph = graphtypes.Subgraph
type GraphStats = graphtypes.GraphStats
type GraphStore = graphtypes.GraphStore
type GraphDecayConfig = graphtypes.GraphDecayConfig

// Extraction types for LLM extraction
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
	MinStrength   float64 `json:"min_strength"`
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

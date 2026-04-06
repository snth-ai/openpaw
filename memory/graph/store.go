package graph

import (
	"time"

	"github.com/openpaw/server/memory/graphtypes"
)

// DefaultGraphDecayConfig returns default decay configuration.
func DefaultGraphDecayConfig() graphtypes.GraphDecayConfig {
	return graphtypes.GraphDecayConfig{
		EdgeDecayFactor: 0.998,
		MinEdgeStrength: 0.1,
		MinMentionCount: 3,
		DecayInterval:   24 * time.Hour,
	}
}

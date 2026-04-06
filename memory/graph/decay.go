package graph

import "log"

// RunDecay выполняет периодическое обслуживание графа.
func RunDecay(store GraphStore, cfg GraphDecayConfig) {
	// 1. Decay edge strengths
	decayed, err := store.DecayEdgeStrengths(cfg.EdgeDecayFactor)
	if err != nil {
		log.Printf("graph decay: edge decay error: %v", err)
	}

	// 2. Prune weak invalidated edges
	pruned, err := store.PruneWeakInvalidatedEdges(cfg.MinEdgeStrength)
	if err != nil {
		log.Printf("graph decay: prune edges error: %v", err)
	}

	// 3. Prune orphan nodes
	orphaned, err := store.PruneOrphanNodes(cfg.MinMentionCount)
	if err != nil {
		log.Printf("graph decay: prune nodes error: %v", err)
	}

	if decayed > 0 || pruned > 0 || orphaned > 0 {
		log.Printf("graph decay: decayed %d edges, pruned %d weak edges, %d orphan nodes", decayed, pruned, orphaned)
	}
}

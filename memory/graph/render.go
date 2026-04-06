package graph

import (
	"fmt"
	"strings"
)

// RenderSubgraph рендерит подграф в текстовый формат для inject в prompt.
func RenderSubgraph(sg *Subgraph) string {
	if sg == nil || len(sg.Edges) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<knowledge>\n")

	// Группируем edges по source node
	nodeEdges := make(map[string][]Edge)
	for _, edge := range sg.Edges {
		nodeEdges[edge.FromID] = append(nodeEdges[edge.FromID], edge)
	}

	// Создаём map для быстрого поиска нод
	nodeMap := make(map[string]*Node)
	for i := range sg.Nodes {
		nodeMap[sg.Nodes[i].ID] = &sg.Nodes[i]
	}

	// Сортируем ноды по mention_count (важные первыми)
	sortedNodes := make([]Node, len(sg.Nodes))
	copy(sortedNodes, sg.Nodes)
	for i := 1; i < len(sortedNodes); i++ {
		for j := i; j > 0 && sortedNodes[j].MentionCount > sortedNodes[j-1].MentionCount; j-- {
			sortedNodes[j], sortedNodes[j-1] = sortedNodes[j-1], sortedNodes[j]
		}
	}

	rendered := make(map[string]bool) // избегаем дублирования нод

	for _, node := range sortedNodes {
		edges := nodeEdges[node.ID]
		if len(edges) == 0 {
			// Проверяем как target
			hasAsTarget := false
			for _, e := range sg.Edges {
				if e.ToID == node.ID {
					hasAsTarget = true
					break
				}
			}
			if !hasAsTarget {
				continue // skip orphan в контексте этого subgraph
			}
		}

		if rendered[node.ID] {
			continue
		}
		rendered[node.ID] = true

		sb.WriteString(fmt.Sprintf("## %s (%s)\n", node.Name, node.Type))
		if node.Summary != "" {
			sb.WriteString(node.Summary + "\n")
		}

		for _, edge := range edges {
			target := nodeMap[edge.ToID]
			if target == nil {
				continue
			}

			line := fmt.Sprintf("- %s → %s", edge.Relation, target.Name)

			// Temporal
			if edge.ValidFrom != nil {
				line += fmt.Sprintf(" [с %s]", edge.ValidFrom.Format("Jan 2006"))
			}

			// Context
			if edge.Context != "" {
				line += fmt.Sprintf(" — %s", edge.Context)
			}

			// Strength indicator для слабых связей
			if edge.Strength < 0.5 {
				line += " (неуверенно)"
			}

			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("</knowledge>")
	return sb.String()
}

// RenderStats рендерит статистику графа.
func RenderStats(stats *GraphStats) string {
	return fmt.Sprintf("Graph: %d nodes, %d edges (%d active, %d invalidated), %d episodes",
		stats.TotalNodes, stats.TotalEdges, stats.ActiveEdges, stats.InvalidatedEdges, stats.TotalEpisodes)
}

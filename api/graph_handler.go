package api

import (
	"encoding/json"
	"net/http"
	"strings"

	graphmem "github.com/openpaw/server/memory/graph"
)

// GraphHandler — REST API для визуализации графа.
type GraphHandler struct {
	store graphmem.GraphStore
}

func NewGraphHandler(store graphmem.GraphStore) *GraphHandler {
	return &GraphHandler{store: store}
}

// Register регистрирует все эндпоинты графа.
func (h *GraphHandler) Register() {
	http.HandleFunc("/api/graph/stats", h.handleStats)
	http.HandleFunc("/api/graph/nodes", h.handleNodes)
	http.HandleFunc("/api/graph/edges", h.handleEdges)
	http.HandleFunc("/api/graph/export", h.handleExport)
	http.HandleFunc("/api/graph/search", h.handleSearch)
	http.HandleFunc("/api/graph/node/", h.handleNode)
	http.HandleFunc("/graph", h.handleVisualization)
}

func (h *GraphHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetStats()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, stats)
}

func (h *GraphHandler) handleNodes(w http.ResponseWriter, r *http.Request) {
	// Возвращаем все ноды (top by mention_count)
	sg, err := h.store.GetSubgraph(nil, false)
	if err != nil {
		// GetSubgraph с nil вернёт пустой — загрузим через stats + перебор
		writeJSON(w, []graphmem.Node{})
		return
	}
	writeJSON(w, sg.Nodes)
}

func (h *GraphHandler) handleEdges(w http.ResponseWriter, r *http.Request) {
	includeInvalidated := r.URL.Query().Get("include_invalidated") == "true"
	_ = includeInvalidated
	// Для полного списка нужен отдельный метод — пока через export
	writeJSON(w, []graphmem.Edge{})
}

func (h *GraphHandler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "missing q parameter", 400)
		return
	}

	tokens := strings.Fields(query)
	nodes, err := h.store.FindNodesByNames(tokens)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Конвертируем []*Node → []Node для JSON
	result := make([]graphmem.Node, len(nodes))
	for i, n := range nodes {
		result[i] = *n
	}
	writeJSON(w, result)
}

func (h *GraphHandler) handleNode(w http.ResponseWriter, r *http.Request) {
	// /api/graph/node/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/graph/node/")
	if id == "" {
		http.Error(w, "missing node id", 400)
		return
	}

	sg, err := h.store.GetSubgraph([]string{id}, r.URL.Query().Get("include_invalidated") == "true")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, sg)
}

// ExportData — формат для vis.js визуализации.
type ExportData struct {
	Nodes []ExportNode `json:"nodes"`
	Edges []ExportEdge `json:"edges"`
	Stats *graphmem.GraphStats `json:"stats"`
}

type ExportNode struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	MentionCount int    `json:"mention_count"`
	Summary      string `json:"summary"`
	Color        string `json:"color"`
	Size         int    `json:"size"`
}

type ExportEdge struct {
	ID            string  `json:"id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	Label         string  `json:"label"`
	RelationGroup string  `json:"relation_group"`
	Strength      float64 `json:"strength"`
	Active        bool    `json:"active"`
	Color         string  `json:"color"`
	Width         int     `json:"width"`
	Dashes        bool    `json:"dashes"`
}

var nodeColors = map[string]string{
	"person":       "#4A90D9",
	"project":      "#7B68EE",
	"place":        "#2ECC71",
	"organization": "#E67E22",
	"concept":      "#9B59B6",
	"event":        "#E74C3C",
	"skill":        "#00BCD4",
	"tool":         "#FF9800",
}

var edgeColors = map[string]string{
	"social":       "#3498DB",
	"professional": "#2ECC71",
	"spatial":      "#F39C12",
	"temporal":     "#9B59B6",
	"causal":       "#E74C3C",
	"preference":   "#E91E63",
	"emotional":    "#FF6B6B",
	"general":      "#95A5A6",
}

func (h *GraphHandler) handleExport(w http.ResponseWriter, r *http.Request) {
	stats, _ := h.store.GetStats()
	nodes, _ := h.store.GetAllNodes()
	edges, _ := h.store.GetAllActiveEdges()

	includeInvalidated := r.URL.Query().Get("include_invalidated") == "true"
	if includeInvalidated {
		// TODO: add GetAllEdges method
	}

	export := ExportData{Stats: stats}

	for _, n := range nodes {
		color := nodeColors[n.Type]
		if color == "" {
			color = "#95A5A6"
		}
		size := 10 + n.MentionCount*3
		if size > 50 {
			size = 50
		}

		export.Nodes = append(export.Nodes, ExportNode{
			ID:           n.ID,
			Label:        n.Name,
			Type:         n.Type,
			MentionCount: n.MentionCount,
			Summary:      n.Summary,
			Color:        color,
			Size:         size,
		})
	}

	for _, e := range edges {
		color := edgeColors[e.RelationGroup]
		if color == "" {
			color = "#95A5A6"
		}
		width := int(e.Strength * 3)
		if width < 1 {
			width = 1
		}

		export.Edges = append(export.Edges, ExportEdge{
			ID:            e.ID,
			From:          e.FromID,
			To:            e.ToID,
			Label:         e.Relation,
			RelationGroup: e.RelationGroup,
			Strength:      e.Strength,
			Active:        e.IsActive(),
			Color:         color,
			Width:         width,
			Dashes:        !e.IsActive(),
		})
	}

	writeJSON(w, export)
}

func (h *GraphHandler) handleVisualization(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/graph.html")
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
}

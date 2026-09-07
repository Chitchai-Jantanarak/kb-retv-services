package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

type fakeGraphQuerier struct {
	results map[string][]map[string]any
	err     error
	lastCID int64
}

func (f *fakeGraphQuerier) Query(_ context.Context, q ports.CypherQuery) ([]map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	if cid, ok := q.Params["company_id"].(int64); ok {
		f.lastCID = cid
	}
	for key, rows := range f.results {
		if strings.Contains(q.Statement, key) {
			return rows, nil
		}
	}
	return nil, nil
}

type fakeVectorInfo struct {
	points int
	dim    int
	exists bool
	err    error
}

func (f *fakeVectorInfo) CollectionInfo(_ context.Context, _ string) (int, int, bool, error) {
	return f.points, f.dim, f.exists, f.err
}

type fakeNamer struct {
	name string
	err  error
}

func (f fakeNamer) CollectionForCompany(_ int64) (string, error) {
	return f.name, f.err
}

func TestKnowledgeStatsHandlerBothAvailable(t *testing.T) {
	graph := &fakeGraphQuerier{results: map[string][]map[string]any{
		"labels(n)[0]": {
			{"label": "Article", "c": int64(40)},
			{"label": "Symptom", "c": int64(56)},
		},
		"type(r)": {
			{"edge_type": "MENTIONS", "c": int64(120)},
			{"edge_type": "SOLVES", "c": int64(94)},
		},
	}}
	vector := &fakeVectorInfo{points: 1842, dim: 768, exists: true}
	namer := fakeNamer{name: "kb_chunks__4"}

	h := NewKnowledgeStatsHandler(graph, vector, namer)
	c, rec := newRequestWithCompany("GET", "/v1/knowledge/stats")
	if err := h.Stats(c); err != nil {
		t.Fatalf("Stats: %v", err)
	}

	var payload struct {
		Data struct {
			Memgraph map[string]any `json:"memgraph"`
			Qdrant   map[string]any `json:"qdrant"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}

	if payload.Data.Memgraph["available"] != true {
		t.Fatalf("memgraph.available = %v", payload.Data.Memgraph["available"])
	}
	if payload.Data.Memgraph["nodes"].(float64) != 96 {
		t.Fatalf("memgraph.nodes = %v, want 96", payload.Data.Memgraph["nodes"])
	}
	if payload.Data.Memgraph["edges"].(float64) != 214 {
		t.Fatalf("memgraph.edges = %v, want 214", payload.Data.Memgraph["edges"])
	}
	if payload.Data.Qdrant["available"] != true {
		t.Fatalf("qdrant.available = %v", payload.Data.Qdrant["available"])
	}
	if payload.Data.Qdrant["vectors"].(float64) != 1842 {
		t.Fatalf("qdrant.vectors = %v, want 1842", payload.Data.Qdrant["vectors"])
	}
	if payload.Data.Qdrant["dim"].(float64) != 768 {
		t.Fatalf("qdrant.dim = %v, want 768", payload.Data.Qdrant["dim"])
	}
	if payload.Data.Qdrant["collection"] != "kb_chunks__4" {
		t.Fatalf("qdrant.collection = %v", payload.Data.Qdrant["collection"])
	}
}

func TestKnowledgeStatsHandlerUnavailable(t *testing.T) {
	h := NewKnowledgeStatsHandler(nil, nil, fakeNamer{})
	c, rec := newRequestWithCompany("GET", "/v1/knowledge/stats")
	if err := h.Stats(c); err != nil {
		t.Fatalf("Stats: %v", err)
	}

	var payload struct {
		Data struct {
			Memgraph map[string]any `json:"memgraph"`
			Qdrant   map[string]any `json:"qdrant"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if payload.Data.Memgraph["available"] != false {
		t.Fatalf("memgraph.available = %v, want false", payload.Data.Memgraph["available"])
	}
	if payload.Data.Qdrant["available"] != false {
		t.Fatalf("qdrant.available = %v, want false", payload.Data.Qdrant["available"])
	}
}

func TestKnowledgeStatsHandlerQdrantErrorIsUnavailable(t *testing.T) {
	vector := &fakeVectorInfo{err: errors.New("boom")}
	h := NewKnowledgeStatsHandler(nil, vector, fakeNamer{name: "kb_chunks__4"})
	c, rec := newRequestWithCompany("GET", "/v1/knowledge/stats")
	if err := h.Stats(c); err != nil {
		t.Fatalf("Stats: %v", err)
	}

	var payload struct {
		Data struct {
			Qdrant map[string]any `json:"qdrant"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if payload.Data.Qdrant["available"] != false {
		t.Fatalf("qdrant.available = %v, want false", payload.Data.Qdrant["available"])
	}
}

func TestKnowledgeGraphHandlerReturnsNodesAndEdges(t *testing.T) {
	graph := &fakeGraphQuerier{results: map[string][]map[string]any{
		"OPTIONAL MATCH": {
			{"id": "n1", "label": "ใบแจ้งหนี้", "type": "Article", "degree": int64(7)},
			{"id": "n2", "label": "Printer offline", "type": "Symptom", "degree": int64(3)},
		},
		"WHERE a.id IN": {
			{"from": "n1", "to": "n2", "type": "RELATED"},
		},
	}}

	h := NewKnowledgeStatsHandler(graph, nil, fakeNamer{})
	c, rec := newRequestWithCompany("GET", "/v1/knowledge/graph?limit=80")
	if err := h.Graph(c); err != nil {
		t.Fatalf("Graph: %v", err)
	}

	var payload struct {
		Data struct {
			Available bool             `json:"available"`
			Nodes     []map[string]any `json:"nodes"`
			Edges     []map[string]any `json:"edges"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !payload.Data.Available {
		t.Fatalf("available = false, want true")
	}
	if len(payload.Data.Nodes) != 2 || payload.Data.Nodes[0]["id"] != "n1" {
		t.Fatalf("nodes = %v", payload.Data.Nodes)
	}
	if payload.Data.Nodes[0]["label"] != "ใบแจ้งหนี้" {
		t.Fatalf("nodes[0].label = %v", payload.Data.Nodes[0]["label"])
	}
	if len(payload.Data.Edges) != 1 || payload.Data.Edges[0]["from"] != "n1" {
		t.Fatalf("edges = %v", payload.Data.Edges)
	}
}

func TestKnowledgeGraphHandlerUnavailableWhenGraphNil(t *testing.T) {
	h := NewKnowledgeStatsHandler(nil, nil, fakeNamer{})
	c, rec := newRequestWithCompany("GET", "/v1/knowledge/graph")
	if err := h.Graph(c); err != nil {
		t.Fatalf("Graph: %v", err)
	}

	var payload struct {
		Data struct {
			Available bool             `json:"available"`
			Nodes     []map[string]any `json:"nodes"`
			Edges     []map[string]any `json:"edges"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if payload.Data.Available {
		t.Fatalf("available = true, want false")
	}
	if len(payload.Data.Nodes) != 0 || len(payload.Data.Edges) != 0 {
		t.Fatalf("expected empty nodes/edges, got %v / %v", payload.Data.Nodes, payload.Data.Edges)
	}
}

func TestClampGraphLimit(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{5, 10},
		{10, 10},
		{80, 80},
		{200, 200},
		{500, 200},
	}
	for _, tc := range cases {
		if got := clampGraphLimit(tc.in); got != tc.want {
			t.Fatalf("clampGraphLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

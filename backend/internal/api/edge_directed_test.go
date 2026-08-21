package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestCreateUndirectedEdgePreservesDirectedFalse(t *testing.T) {
	h := newTestServer(t)
	sourceID := createNode(t, h, "客户甲", "person", nil)
	targetID := createNode(t, h, "客户乙", "person", nil)

	status, resp := call(t, h, http.MethodPost, "/api/v1/edges", map[string]any{
		"source_id": sourceID,
		"target_id": targetID,
		"relation":  "associates_with",
		"directed":  false,
	})
	if status != http.StatusCreated {
		t.Fatalf("创建无向关系期望 201，实际 %d: %+v", status, resp)
	}

	var created graph.Edge
	dataAs(t, resp, &created)
	if created.Directed {
		t.Fatalf("显式 directed=false 后创建结果仍为有向关系: %+v", created)
	}

	status, resp = call(t, h, http.MethodGet, "/api/v1/edges/"+created.ID, nil)
	if status != http.StatusOK {
		t.Fatalf("读取新建关系期望 200，实际 %d: %+v", status, resp)
	}
	var stored graph.Edge
	dataAs(t, resp, &stored)
	if stored.Directed {
		t.Errorf("持久化后的关系没有保留 directed=false: %+v", stored)
	}
}

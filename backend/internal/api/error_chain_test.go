package api

import (
	"net/http"
	"testing"
)

func TestUpdateEdgeConflictPreservesDomainError(t *testing.T) {
	h := newTestServer(t)
	source := createNode(t, h, "订单服务", "application", nil)
	target := createNode(t, h, "订单库", "database", nil)

	createEdge(t, h, source, target, "reads_from", 1)
	edgeID := createEdge(t, h, source, target, "writes_to", 1)

	status, resp := call(t, h, http.MethodPut, "/api/v1/edges/"+edgeID, map[string]any{
		"relation": "reads_from",
		"reason":   "纠正关系类型",
	})
	if status != http.StatusConflict {
		t.Fatalf("关系类型与已有关系冲突应返回 409，实际 status=%d resp=%+v", status, resp)
	}
	if resp.Code != CodeEdgeExists {
		t.Fatalf("关系冲突应返回业务码 %d，实际 %d", CodeEdgeExists, resp.Code)
	}
}

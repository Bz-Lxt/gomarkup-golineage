package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestNodeImpactHonorsMaxDepth(t *testing.T) {
	h := newTestServer(t)
	root := createNode(t, h, "结算入口", "application", nil)
	direct := createNode(t, h, "账务服务", "application", nil)
	indirect := createNode(t, h, "归档数据库", "database", nil)
	createEdge(t, h, root, direct, "calls", 1)
	createEdge(t, h, direct, indirect, "writes_to", 1)

	status, resp := call(t, h, http.MethodGet,
		"/api/v1/nodes/"+root+"/impact?max_depth=1", nil)
	if status != http.StatusOK {
		t.Fatalf("impact request returned status %d: %+v", status, resp)
	}

	var impact graph.ImpactSummary
	dataAs(t, resp, &impact)
	if impact.TotalDownstream != 1 {
		t.Fatalf("max_depth=1 should include only the direct downstream asset, got %d", impact.TotalDownstream)
	}
	if impact.DirectDownstrem != 1 {
		t.Fatalf("expected one direct downstream asset, got %d", impact.DirectDownstrem)
	}
	if impact.MaxDepthReached != 1 {
		t.Fatalf("max_depth=1 should not report a reached depth of %d", impact.MaxDepthReached)
	}
}

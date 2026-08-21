package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestNeighborsHonorsRequestedHops(t *testing.T) {
	h := newTestServer(t)
	a := createNode(t, h, "入口服务", "application", nil)
	b := createNode(t, h, "订单服务", "application", nil)
	c := createNode(t, h, "订单库", "database", nil)
	createEdge(t, h, a, b, "calls", 1)
	createEdge(t, h, b, c, "reads_from", 1)

	status, resp := call(t, h, http.MethodGet,
		"/api/v1/graph/neighbors?id="+a+"&hops=2", nil)
	if status != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %+v", status, resp)
	}

	var got graph.TraverseResult
	dataAs(t, resp, &got)
	if got.VisitedCount != 3 {
		t.Fatalf("请求两跳邻居应返回起点、一跳和二跳节点共 3 个，实际 %d", got.VisitedCount)
	}
	foundSecondHop := false
	for _, n := range got.Nodes {
		if n.ID == c {
			foundSecondHop = true
			break
		}
	}
	if !foundSecondHop {
		t.Error("两跳邻居响应中缺少二跳节点")
	}
}

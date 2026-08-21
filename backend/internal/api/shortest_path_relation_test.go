package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestShortestPathHonorsRelationFilter(t *testing.T) {
	h := newTestServer(t)
	start := createNode(t, h, "起点", "application", nil)
	callsHop := createNode(t, h, "调用链中转", "application", nil)
	readsHop := createNode(t, h, "读取链中转", "database", nil)
	end := createNode(t, h, "终点", "application", nil)

	createEdge(t, h, start, callsHop, "calls", 5)
	createEdge(t, h, callsHop, end, "calls", 5)
	createEdge(t, h, start, readsHop, "reads_from", 1)
	createEdge(t, h, readsHop, end, "reads_from", 1)

	path := fmt.Sprintf(
		"/api/v1/graph/shortest-path?from=%s&to=%s&direction=out&relation=calls",
		start, end,
	)
	status, resp := call(t, h, http.MethodGet, path, nil)
	if status != http.StatusOK {
		t.Fatalf("查询最短路径失败: status=%d resp=%+v", status, resp)
	}

	var result graph.PathResult
	dataAs(t, resp, &result)
	if !result.Found {
		t.Fatal("指定 calls 关系后应能找到路径")
	}
	if len(result.Nodes) != 3 || result.Nodes[1].ID != callsHop {
		t.Fatalf("路径应只使用 calls 关系，实际节点为 %+v", result.Nodes)
	}
	if result.TotalCost != 10 {
		t.Fatalf("calls 路径总代价应为 10，实际为 %v", result.TotalCost)
	}
	for _, edge := range result.Edges {
		if edge.Relation != graph.RelCalls {
			t.Fatalf("路径包含未请求的关系类型 %q", edge.Relation)
		}
	}
}

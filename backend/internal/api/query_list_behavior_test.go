package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestNodeSearchAcceptsRepeatedTypeParameters(t *testing.T) {
	h := newTestServer(t)
	createNode(t, h, "订单服务", "application", nil)
	createNode(t, h, "订单库", "database", nil)
	createNode(t, h, "缓存集群", "middleware", nil)

	status, resp := call(t, h, http.MethodGet,
		"/api/v1/nodes?type=application&type=database", nil)
	if status != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %+v", status, resp)
	}

	var result struct {
		Items []*graph.Node `json:"items"`
		Count int           `json:"count"`
	}
	dataAs(t, resp, &result)
	if result.Count != 2 || len(result.Items) != 2 {
		t.Fatalf("重复 type 参数应合并为多类型筛选，期望 2 个结果，实际 count=%d items=%d: %+v",
			result.Count, len(result.Items), result.Items)
	}

	seen := make(map[graph.NodeType]bool, len(result.Items))
	for _, item := range result.Items {
		seen[item.Type] = true
	}
	if !seen[graph.NodeTypeApplication] || !seen[graph.NodeTypeDatabase] {
		t.Fatalf("期望同时返回 application 与 database，实际类型集合: %v", seen)
	}
}

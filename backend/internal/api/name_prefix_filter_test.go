package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestListNodesFiltersByNamePrefix(t *testing.T) {
	h := newTestServer(t)
	createNode(t, h, "订单服务", "application", nil)
	createNode(t, h, "订单数据库", "database", nil)
	createNode(t, h, "历史订单归档", "database", nil)

	status, resp := call(t, h, http.MethodGet, "/api/v1/nodes?name_prefix=订单", nil)
	if status != http.StatusOK {
		t.Fatalf("按名称前缀检索应返回 200，实际 %d: %+v", status, resp)
	}

	var result struct {
		Items []*graph.Node `json:"items"`
		Count int           `json:"count"`
	}
	dataAs(t, resp, &result)
	if result.Count != 2 || len(result.Items) != 2 {
		t.Fatalf("名称前缀为「订单」时应只返回 2 个资产，实际 count=%d items=%v", result.Count, result.Items)
	}
	for _, node := range result.Items {
		if node.Name != "订单服务" && node.Name != "订单数据库" {
			t.Errorf("返回了名称不以「订单」开头的资产 %q", node.Name)
		}
	}
}

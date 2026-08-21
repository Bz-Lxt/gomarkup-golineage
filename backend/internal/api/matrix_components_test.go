package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestMatrixAnalysisKeepsDisconnectedComponentsDistinct(t *testing.T) {
	h := newTestServer(t)

	pairs := make([][2]string, 0, 3)
	for _, names := range [][2]string{
		{"订单入口", "订单服务"},
		{"库存入口", "库存服务"},
		{"支付入口", "支付服务"},
	} {
		left := createNode(t, h, names[0], "application", nil)
		right := createNode(t, h, names[1], "application", nil)
		createEdge(t, h, left, right, "calls", 1)
		pairs = append(pairs, [2]string{left, right})
	}

	status, resp := call(t, h, http.MethodPost, "/api/v1/graph/matrix", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("矩阵分析失败: status=%d resp=%+v", status, resp)
	}
	var analysis graph.MatrixAnalysis
	dataAs(t, resp, &analysis)

	if analysis.ComponentCount != len(pairs) {
		t.Fatalf("期望 %d 个连通分量，实际 %d: %v", len(pairs), analysis.ComponentCount, analysis.Components)
	}
	seen := make(map[string]int, len(pairs)*2)
	for _, component := range analysis.Components {
		if len(component) != 2 {
			t.Fatalf("每个独立调用对都应包含 2 个节点，实际分量: %v", analysis.Components)
		}
		for _, id := range component {
			seen[id]++
		}
	}
	for _, pair := range pairs {
		for _, id := range pair {
			if seen[id] != 1 {
				t.Errorf("节点 %s 应恰好出现在一个连通分量中，实际出现 %d 次；全部分量: %v", id, seen[id], analysis.Components)
			}
		}
	}
}

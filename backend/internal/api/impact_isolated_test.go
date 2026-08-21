package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestIsolatedNodeImpactReturnsZeroSummary(t *testing.T) {
	h := newTestServer(t)
	id := createNode(t, h, "待接入服务", "application", nil)

	status, resp := call(t, h, http.MethodGet, "/api/v1/nodes/"+id+"/impact", nil)
	if status != http.StatusOK {
		t.Fatalf("孤立资产影响评估期望 200，实际 %d: %+v", status, resp)
	}
	if resp.Data == nil {
		t.Fatal("孤立资产影响评估应返回零值统计对象，实际 data=null")
	}

	var got graph.ImpactSummary
	dataAs(t, resp, &got)
	if got.NodeID != id || got.NodeName != "待接入服务" {
		t.Errorf("影响评估应标识被查询资产，实际 id=%q name=%q", got.NodeID, got.NodeName)
	}
	if got.DirectDownstrem != 0 || got.TotalDownstream != 0 ||
		got.DirectUpstream != 0 || got.TotalUpstream != 0 || got.MaxDepthReached != 0 {
		t.Errorf("孤立资产的影响统计应全部为 0，实际 %+v", got)
	}
	if got.ByType == nil {
		t.Error("孤立资产的按类型统计应为空对象而不是 null")
	}
}

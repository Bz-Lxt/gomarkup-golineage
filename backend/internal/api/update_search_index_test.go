package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/service"
)

func TestUpdatedNodeRemainsSearchableByType(t *testing.T) {
	h := newTestServer(t)
	id := createNode(t, h, "结算服务", "application", map[string]any{"owner": "张三"})

	status, resp := call(t, h, http.MethodPut, "/api/v1/nodes/"+id, map[string]any{
		"properties": map[string]any{"owner": "李四"},
		"reason":     "负责人调整",
	})
	if status != http.StatusOK {
		t.Fatalf("更新资产失败: status=%d resp=%+v", status, resp)
	}

	status, resp = call(t, h, http.MethodGet, "/api/v1/nodes/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("更新后按 ID 读取资产失败: status=%d resp=%+v", status, resp)
	}
	var detail service.NodeDetail
	dataAs(t, resp, &detail)
	if detail.Node.Properties["owner"] != "李四" {
		t.Fatalf("详情未返回更新后的负责人: %+v", detail.Node.Properties)
	}

	status, resp = call(t, h, http.MethodGet, "/api/v1/nodes?type=application", nil)
	if status != http.StatusOK {
		t.Fatalf("按类型检索资产失败: status=%d resp=%+v", status, resp)
	}
	var result struct {
		Items []*graph.Node `json:"items"`
	}
	dataAs(t, resp, &result)
	for _, node := range result.Items {
		if node.ID == id {
			return
		}
	}
	t.Fatalf("更新后的 application 资产应仍可按类型检索到，实际返回: %+v", result.Items)
}

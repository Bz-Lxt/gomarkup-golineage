package api

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestAllPathsHonorsMaxPathsQuery(t *testing.T) {
	h := newTestServer(t)

	source := createNode(t, h, "订单源", "database", nil)
	middleA := createNode(t, h, "清洗任务 A", "application", nil)
	middleB := createNode(t, h, "清洗任务 B", "application", nil)
	target := createNode(t, h, "经营报表", "application", nil)
	createEdge(t, h, source, middleA, "calls", 1)
	createEdge(t, h, middleA, target, "calls", 1)
	createEdge(t, h, source, middleB, "calls", 1)
	createEdge(t, h, middleB, target, "calls", 1)

	path := fmt.Sprintf("/api/v1/graph/all-paths?from=%s&to=%s&max_paths=1",
		url.QueryEscape(source), url.QueryEscape(target))
	status, resp := call(t, h, http.MethodGet, path, nil)
	if status != http.StatusOK {
		t.Fatalf("查询全路径失败: status=%d resp=%+v", status, resp)
	}

	var result graph.AllPathsResult
	dataAs(t, resp, &result)
	if len(result.Paths) != 1 {
		t.Fatalf("max_paths=1 时应只返回 1 条路径，实际返回 %d 条", len(result.Paths))
	}
	if !result.Truncated {
		t.Fatal("存在更多可达路径时应标记 truncated=true")
	}
}

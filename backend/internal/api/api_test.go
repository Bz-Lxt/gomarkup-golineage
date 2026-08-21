package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
	"github.com/alkaid/golineage/internal/service"
)

// newTestServer 装配一套完全在内存中运行的服务实例。
// 事件存储用内存实现，因此这些测试不依赖数据库，可离线执行。
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{
		GraphMaxDepth:    10,
		GraphMaxPaths:    100,
		GraphMaxNodes:    5000,
		GraphQueryTimout: 5 * time.Second,
		SnapshotInterval: 0,
		CORSAllowOrigin:  []string{"http://localhost:27430"},
	}
	g := graph.New(graph.Limits{MaxDepth: cfg.GraphMaxDepth, MaxPaths: cfg.GraphMaxPaths, MaxNodes: cfg.GraphMaxNodes})
	svc := service.New(repository.NewMemoryGraphAdapter(g), eventstore.NewMemoryStore(), cfg)
	return NewHandler(svc).Router(cfg.CORSAllowOrigin, 10*time.Second)
}

// call 发起请求并解析统一响应包络。
func call(t *testing.T, h http.Handler, method, path string, body any) (int, Response) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp Response
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应失败(%s %s): %v，原文: %s", method, path, err, rec.Body.String())
		}
	}
	return rec.Code, resp
}

// dataAs 把响应中的 data 字段转换为目标结构。
func dataAs(t *testing.T, resp Response, dst any) {
	t.Helper()
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("重新序列化 data 失败: %v", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("解析 data 失败: %v，原文: %s", err, string(raw))
	}
}

// createNode 创建节点并返回其 ID。
func createNode(t *testing.T, h http.Handler, name, typ string, props map[string]any) string {
	t.Helper()
	status, resp := call(t, h, http.MethodPost, "/api/v1/nodes", map[string]any{
		"name": name, "type": typ, "properties": props, "reason": "接口测试",
	})
	if status != http.StatusCreated {
		t.Fatalf("创建节点 %s 失败: status=%d resp=%+v", name, status, resp)
	}
	var n graph.Node
	dataAs(t, resp, &n)
	if n.ID == "" {
		t.Fatalf("创建节点未返回 ID: %+v", resp)
	}
	return n.ID
}

func createEdge(t *testing.T, h http.Handler, src, dst, rel string, weight float64) string {
	t.Helper()
	status, resp := call(t, h, http.MethodPost, "/api/v1/edges", map[string]any{
		"source_id": src, "target_id": dst, "relation": rel, "weight": weight, "reason": "接口测试",
	})
	if status != http.StatusCreated {
		t.Fatalf("创建关系失败: status=%d resp=%+v", status, resp)
	}
	var e graph.Edge
	dataAs(t, resp, &e)
	return e.ID
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t)
	status, resp := call(t, h, http.MethodGet, "/healthz", nil)
	if status != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %+v", status, resp)
	}
	var hs service.HealthStatus
	dataAs(t, resp, &hs)
	if hs.Status != "ok" {
		t.Errorf("期望健康状态 ok，实际 %s", hs.Status)
	}
	if hs.Adapter == "" {
		t.Error("健康检查应报告图适配器标识")
	}
}

func TestTraceIDHeader(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Header().Get(traceIDHeader) == "" {
		t.Error("响应应携带 X-Trace-Id 头，便于报障时定位")
	}
}

func TestNodeCRUD(t *testing.T) {
	h := newTestServer(t)

	id := createNode(t, h, "订单服务", "application", map[string]any{"owner": "张三"})

	t.Run("读取详情", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/nodes/"+id, nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var d service.NodeDetail
		dataAs(t, resp, &d)
		if d.Node.Name != "订单服务" {
			t.Errorf("期望名称「订单服务」，实际 %s", d.Node.Name)
		}
		if d.EventCount != 1 {
			t.Errorf("新建节点应有 1 条变更记录，实际 %d", d.EventCount)
		}
	})

	t.Run("动态新增属性", func(t *testing.T) {
		status, resp := call(t, h, http.MethodPut, "/api/v1/nodes/"+id, map[string]any{
			"properties": map[string]any{"ip": "10.0.0.5", "risk_level": "high"},
			"reason":     "补充资产信息",
		})
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var n graph.Node
		dataAs(t, resp, &n)
		// 合并语义：原有 owner 必须保留。
		if n.Properties["owner"] != "张三" {
			t.Errorf("合并更新不应丢失原有属性，实际 %v", n.Properties)
		}
		if n.Properties["ip"] != "10.0.0.5" || n.Properties["risk_level"] != "high" {
			t.Errorf("新属性未写入: %v", n.Properties)
		}
	})

	t.Run("传 null 删除属性", func(t *testing.T) {
		status, resp := call(t, h, http.MethodPut, "/api/v1/nodes/"+id, map[string]any{
			"properties": map[string]any{"ip": nil},
			"reason":     "下线该 IP",
		})
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var n graph.Node
		dataAs(t, resp, &n)
		if _, still := n.Properties["ip"]; still {
			t.Errorf("值为 null 的键应被删除，实际 %v", n.Properties)
		}
	})

	t.Run("整体替换属性", func(t *testing.T) {
		status, _ := call(t, h, http.MethodPut, "/api/v1/nodes/"+id, map[string]any{
			"properties":         map[string]any{"only": "one"},
			"replace_properties": true,
		})
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		_, resp := call(t, h, http.MethodGet, "/api/v1/nodes/"+id, nil)
		var d service.NodeDetail
		dataAs(t, resp, &d)
		if len(d.Node.Properties) != 1 {
			t.Errorf("替换模式应只剩 1 个属性，实际 %v", d.Node.Properties)
		}
	})

	t.Run("删除", func(t *testing.T) {
		status, resp := call(t, h, http.MethodDelete, "/api/v1/nodes/"+id, map[string]any{
			"reason": "服务下线",
		})
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		status, _ = call(t, h, http.MethodGet, "/api/v1/nodes/"+id, nil)
		if status != http.StatusNotFound {
			t.Errorf("删除后再查应返回 404，实际 %d", status)
		}
	})
}

func TestNodeValidationErrors(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		name     string
		body     map[string]any
		wantHTTP int
		wantCode int
	}{
		{"类型非法", map[string]any{"name": "X", "type": "unknown"}, http.StatusBadRequest, CodeValidation},
		{"名称为空", map[string]any{"name": "  ", "type": "server"}, http.StatusBadRequest, CodeValidation},
		{"属性键非法", map[string]any{"name": "X", "type": "server", "properties": map[string]any{"bad key!": 1}}, http.StatusBadRequest, CodeValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, resp := call(t, h, http.MethodPost, "/api/v1/nodes", tc.body)
			if status != tc.wantHTTP {
				t.Fatalf("期望 HTTP %d，实际 %d: %+v", tc.wantHTTP, status, resp)
			}
			if resp.Code != tc.wantCode {
				t.Errorf("期望业务码 %d，实际 %d", tc.wantCode, resp.Code)
			}
		})
	}

	t.Run("请求体不是合法 JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", strings.NewReader(`{"name":`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("期望 400，实际 %d", rec.Code)
		}
	})

	t.Run("节点不存在", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/nodes/not-exist", nil)
		if status != http.StatusNotFound || resp.Code != CodeNodeNotFound {
			t.Errorf("期望 404/%d，实际 %d/%d", CodeNodeNotFound, status, resp.Code)
		}
	})
}

func TestEdgeCRUD(t *testing.T) {
	h := newTestServer(t)
	a := createNode(t, h, "应用A", "application", nil)
	b := createNode(t, h, "数据库B", "database", nil)

	edgeID := createEdge(t, h, a, b, "reads_from", 2.5)

	t.Run("重复关系被拒", func(t *testing.T) {
		status, resp := call(t, h, http.MethodPost, "/api/v1/edges", map[string]any{
			"source_id": a, "target_id": b, "relation": "reads_from",
		})
		if status != http.StatusConflict || resp.Code != CodeEdgeExists {
			t.Errorf("期望 409/%d，实际 %d/%d", CodeEdgeExists, status, resp.Code)
		}
	})

	t.Run("端点不存在被拒", func(t *testing.T) {
		status, resp := call(t, h, http.MethodPost, "/api/v1/edges", map[string]any{
			"source_id": a, "target_id": "ghost", "relation": "calls",
		})
		if status != http.StatusNotFound || resp.Code != CodeNodeNotFound {
			t.Errorf("期望 404/%d，实际 %d/%d", CodeNodeNotFound, status, resp.Code)
		}
	})

	t.Run("负权重被拒", func(t *testing.T) {
		w := -1.0
		status, resp := call(t, h, http.MethodPost, "/api/v1/edges", map[string]any{
			"source_id": b, "target_id": a, "relation": "calls", "weight": w,
		})
		if status != http.StatusBadRequest || resp.Code != CodeValidation {
			t.Errorf("负权重会破坏 Dijkstra 正确性，必须拒绝，实际 %d/%d", status, resp.Code)
		}
	})

	t.Run("修改权重", func(t *testing.T) {
		status, resp := call(t, h, http.MethodPut, "/api/v1/edges/"+edgeID, map[string]any{
			"weight": 9.0, "reason": "链路劣化",
		})
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var e graph.Edge
		dataAs(t, resp, &e)
		if e.Weight != 9 {
			t.Errorf("期望权重 9，实际 %v", e.Weight)
		}
	})

	t.Run("解除关系", func(t *testing.T) {
		status, _ := call(t, h, http.MethodDelete, "/api/v1/edges/"+edgeID, map[string]any{
			"reason": "A 应用不再读取 B 数据库",
		})
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		status, resp := call(t, h, http.MethodGet, "/api/v1/edges/"+edgeID, nil)
		if status != http.StatusNotFound || resp.Code != CodeEdgeNotFound {
			t.Errorf("期望 404/%d，实际 %d/%d", CodeEdgeNotFound, status, resp.Code)
		}
	})
}

// TestCascadeDeleteViaAPI 删除节点必须级联解除其关系，并把级联结果回报给调用方。
func TestCascadeDeleteViaAPI(t *testing.T) {
	h := newTestServer(t)
	a := createNode(t, h, "网关", "api", nil)
	b := createNode(t, h, "用户服务", "application", nil)
	c := createNode(t, h, "用户库", "database", nil)
	createEdge(t, h, a, b, "calls", 1)
	createEdge(t, h, b, c, "reads_from", 1)

	status, resp := call(t, h, http.MethodDelete, "/api/v1/nodes/"+b, map[string]any{"reason": "服务下线"})
	if status != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %+v", status, resp)
	}
	var res service.DeleteNodeResult
	dataAs(t, resp, &res)
	if len(res.CascadedEdges) != 2 {
		t.Fatalf("期望级联解除 2 条关系，实际 %d", len(res.CascadedEdges))
	}

	// 拓扑中不应残留悬空边。
	_, topoResp := call(t, h, http.MethodGet, "/api/v1/graph", nil)
	var topo graph.Topology
	dataAs(t, topoResp, &topo)
	if len(topo.Edges) != 0 {
		t.Errorf("级联删除后不应残留边，实际 %d 条", len(topo.Edges))
	}
	if len(topo.Nodes) != 2 {
		t.Errorf("期望剩 2 个节点，实际 %d", len(topo.Nodes))
	}
}

func TestGraphQueries(t *testing.T) {
	h := newTestServer(t)

	// 构造 A --1--> B --2--> D 与 A --4--> C --1--> D 的双路径图。
	a := createNode(t, h, "应用A", "application", nil)
	b := createNode(t, h, "服务B", "application", nil)
	c := createNode(t, h, "服务C", "application", nil)
	d := createNode(t, h, "数据库D", "database", nil)
	createEdge(t, h, a, b, "calls", 1)
	createEdge(t, h, b, d, "reads_from", 2)
	createEdge(t, h, a, c, "calls", 4)
	createEdge(t, h, c, d, "reads_from", 1)

	t.Run("最短路径", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet,
			fmt.Sprintf("/api/v1/graph/shortest-path?from=%s&to=%s&direction=out", a, d), nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var p graph.PathResult
		dataAs(t, resp, &p)
		if !p.Found {
			t.Fatal("A→D 应当可达")
		}
		if p.TotalCost != 3 {
			t.Errorf("期望最小代价 3（A→B→D），实际 %v", p.TotalCost)
		}
		if p.Hops != 2 {
			t.Errorf("期望 2 跳，实际 %d", p.Hops)
		}
	})

	t.Run("全路径枚举", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet,
			fmt.Sprintf("/api/v1/graph/all-paths?from=%s&to=%s&direction=out", a, d), nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var res graph.AllPathsResult
		dataAs(t, resp, &res)
		if len(res.Paths) != 2 {
			t.Errorf("期望 2 条路径，实际 %d", len(res.Paths))
		}
	})

	t.Run("邻居子图", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/graph/neighbors?id="+b+"&hops=1", nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var res graph.TraverseResult
		dataAs(t, resp, &res)
		if res.VisitedCount != 3 {
			t.Errorf("B 的一跳邻居应为 A、D 加自身共 3 个，实际 %d", res.VisitedCount)
		}
	})

	t.Run("血缘分析", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/graph/lineage?root="+b, nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var res graph.LineageResult
		dataAs(t, resp, &res)
		if res.UpstreamCount != 1 || res.DownstreamCount != 1 {
			t.Errorf("B 应有 1 个上游与 1 个下游，实际 上游%d 下游%d", res.UpstreamCount, res.DownstreamCount)
		}
	})

	t.Run("DFS 遍历", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet,
			"/api/v1/graph/traverse?algorithm=dfs&start="+a+"&direction=out", nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var res graph.TraverseResult
		dataAs(t, resp, &res)
		if res.Algorithm != "dfs" || res.VisitedCount != 4 {
			t.Errorf("期望 dfs 访问 4 个节点，实际 %s/%d", res.Algorithm, res.VisitedCount)
		}
	})

	t.Run("邻接矩阵分析", func(t *testing.T) {
		status, resp := call(t, h, http.MethodPost, "/api/v1/graph/matrix", map[string]any{
			"node_ids": []string{a, b, c, d},
		})
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var m graph.MatrixAnalysis
		dataAs(t, resp, &m)
		if len(m.Matrix) != 4 {
			t.Errorf("期望 4x4 矩阵，实际 %d 行", len(m.Matrix))
		}
		if m.ComponentCount != 1 {
			t.Errorf("图是连通的，期望 1 个分量，实际 %d", m.ComponentCount)
		}
	})

	t.Run("非法算法名被拒", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/graph/traverse?algorithm=astar&start="+a, nil)
		if status != http.StatusBadRequest || resp.Code != CodeValidation {
			t.Errorf("期望 400/%d，实际 %d/%d", CodeValidation, status, resp.Code)
		}
	})

	t.Run("非法关系类型被拒", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/graph/neighbors?id="+a+"&relation=bogus", nil)
		if status != http.StatusBadRequest {
			t.Errorf("期望 400，实际 %d: %+v", status, resp)
		}
	})

	t.Run("缺少必填参数被拒", func(t *testing.T) {
		status, _ := call(t, h, http.MethodGet, "/api/v1/graph/shortest-path?from="+a, nil)
		if status != http.StatusBadRequest {
			t.Errorf("缺少 to 参数应返回 400，实际 %d", status)
		}
	})
}

func TestSearchAndMetadata(t *testing.T) {
	h := newTestServer(t)
	createNode(t, h, "订单服务", "application", map[string]any{"risk_level": "high"})
	createNode(t, h, "订单库", "database", map[string]any{"risk_level": "high"})
	createNode(t, h, "缓存集群", "middleware", nil)

	t.Run("按关键字检索", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/nodes?keyword=订单", nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var res struct {
			Count int `json:"count"`
		}
		dataAs(t, resp, &res)
		if res.Count != 2 {
			t.Errorf("期望命中 2 个，实际 %d", res.Count)
		}
	})

	t.Run("按类型检索", func(t *testing.T) {
		_, resp := call(t, h, http.MethodGet, "/api/v1/nodes?type=database", nil)
		var res struct {
			Count int `json:"count"`
		}
		dataAs(t, resp, &res)
		if res.Count != 1 {
			t.Errorf("期望命中 1 个数据库，实际 %d", res.Count)
		}
	})

	t.Run("按属性检索", func(t *testing.T) {
		_, resp := call(t, h, http.MethodGet, "/api/v1/nodes?prop_key=risk_level&prop_value=high", nil)
		var res struct {
			Count int `json:"count"`
		}
		dataAs(t, resp, &res)
		if res.Count != 2 {
			t.Errorf("期望命中 2 个高风险资产，实际 %d", res.Count)
		}
	})

	t.Run("元数据下发枚举", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/meta", nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var meta service.Metadata
		dataAs(t, resp, &meta)
		if len(meta.NodeTypes) != 8 {
			t.Errorf("期望下发 8 种节点类型，实际 %d", len(meta.NodeTypes))
		}
		if len(meta.RelationTypes) != 8 {
			t.Errorf("期望下发 8 种关系类型，实际 %d", len(meta.RelationTypes))
		}
		if len(meta.EventTypes) != 6 {
			t.Errorf("期望下发 6 种事件类型，实际 %d", len(meta.EventTypes))
		}
	})
}

// TestTimelineEndpoints 时间轴回溯：解除关系后，回到历史时刻仍应看到它。
func TestTimelineEndpoints(t *testing.T) {
	h := newTestServer(t)
	a := createNode(t, h, "应用A", "application", nil)
	b := createNode(t, h, "数据库B", "database", nil)
	edgeID := createEdge(t, h, a, b, "reads_from", 1)

	// 事件时间戳精度为秒级，这里稍作等待以确保历史点与当前点可区分。
	time.Sleep(1100 * time.Millisecond)
	beforeDelete := time.Now()
	time.Sleep(1100 * time.Millisecond)

	status, _ := call(t, h, http.MethodDelete, "/api/v1/edges/"+edgeID, map[string]any{
		"reason": "A 应用不再读取 B 数据库",
	})
	if status != http.StatusOK {
		t.Fatalf("解除关系失败: %d", status)
	}

	t.Run("变更流水", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/timeline/events?limit=20", nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var page service.EventPage
		dataAs(t, resp, &page)
		if page.Total != 4 {
			t.Errorf("期望 4 条流水（2 建点 + 1 建边 + 1 删边），实际 %d", page.Total)
		}
		if len(page.Items) > 0 && page.Items[0].TypeLabel == "" {
			t.Error("流水应携带中文事件标签供前端直接展示")
		}
	})

	t.Run("按实体过滤流水", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/nodes/"+a+"/events", nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var page service.EventPage
		dataAs(t, resp, &page)
		if page.Total != 1 {
			t.Errorf("节点 A 应有 1 条变更记录，实际 %d", page.Total)
		}
	})

	t.Run("历史快照仍含已解除的关系", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet,
			"/api/v1/timeline/snapshot?at="+url.QueryEscape(beforeDelete.Format(time.RFC3339)), nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var hist service.HistoricalTopology
		dataAs(t, resp, &hist)
		if len(hist.Topology.Edges) != 1 {
			t.Errorf("删除前的历史时刻应仍有 1 条关系，实际 %d", len(hist.Topology.Edges))
		}
	})

	// RFC3339 的 "+08:00" 若未经 URL 编码，服务端会把 "+" 解回空格。
	// 这个陷阱前端极易踩中，服务端必须兼容而不是直接报错。
	t.Run("未编码的时区偏移仍可解析", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet,
			"/api/v1/timeline/snapshot?at="+beforeDelete.Format(time.RFC3339), nil)
		if status != http.StatusOK {
			t.Fatalf("未编码的 + 号应被兼容，实际 %d: %+v", status, resp)
		}
		var hist service.HistoricalTopology
		dataAs(t, resp, &hist)
		if len(hist.Topology.Edges) != 1 {
			t.Errorf("期望仍有 1 条关系，实际 %d", len(hist.Topology.Edges))
		}
	})

	t.Run("当前拓扑已无该关系", func(t *testing.T) {
		_, resp := call(t, h, http.MethodGet, "/api/v1/graph", nil)
		var topo graph.Topology
		dataAs(t, resp, &topo)
		if len(topo.Edges) != 0 {
			t.Errorf("当前拓扑不应再有关系，实际 %d 条", len(topo.Edges))
		}
	})

	t.Run("拓扑对比", func(t *testing.T) {
		now := time.Now().Add(time.Second)
		status, resp := call(t, h, http.MethodGet, fmt.Sprintf("/api/v1/timeline/diff?from=%s&to=%s",
			url.QueryEscape(beforeDelete.Format(time.RFC3339)),
			url.QueryEscape(now.Format(time.RFC3339))), nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d: %+v", status, resp)
		}
		var diff eventstore.TopologyDiff
		dataAs(t, resp, &diff)
		if diff.Summary.EdgesRemoved != 1 {
			t.Errorf("期望识别出 1 条被解除的关系，实际 %+v", diff.Summary)
		}
	})

	t.Run("时间轴范围", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/timeline/bounds", nil)
		if status != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", status)
		}
		var b service.TimelineBounds
		dataAs(t, resp, &b)
		if !b.Available || b.EventCount != 4 {
			t.Errorf("期望时间轴可用且含 4 条事件，实际 %+v", b)
		}
	})

	t.Run("时间格式非法被拒", func(t *testing.T) {
		status, resp := call(t, h, http.MethodGet, "/api/v1/timeline/snapshot?at=not-a-time", nil)
		if status != http.StatusBadRequest || resp.Code != CodeValidation {
			t.Errorf("期望 400/%d，实际 %d/%d", CodeValidation, status, resp.Code)
		}
	})

	t.Run("缺少 at 参数被拒", func(t *testing.T) {
		status, _ := call(t, h, http.MethodGet, "/api/v1/timeline/snapshot", nil)
		if status != http.StatusBadRequest {
			t.Errorf("期望 400，实际 %d", status)
		}
	})
}

func TestRouteNotFound(t *testing.T) {
	h := newTestServer(t)
	status, resp := call(t, h, http.MethodGet, "/api/v1/nonexistent", nil)
	if status != http.StatusNotFound || resp.Code != CodeRouteMissing {
		t.Errorf("期望 404/%d，实际 %d/%d", CodeRouteMissing, status, resp.Code)
	}
}

func TestCORSWhitelist(t *testing.T) {
	h := newTestServer(t)

	t.Run("白名单内放行", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Origin", "http://localhost:27430")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:27430" {
			t.Errorf("白名单来源应被放行，实际 %q", got)
		}
	})

	t.Run("白名单外不放行", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Origin", "http://evil.example.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("非白名单来源不应获得放行头，实际 %q", got)
		}
	})
}

// TestRestartConsistency 服务重启后，从事件日志重建的拓扑必须与重启前完全一致。
func TestRestartConsistency(t *testing.T) {
	cfg := &config.Config{
		GraphMaxDepth: 10, GraphMaxPaths: 100, GraphMaxNodes: 5000,
		GraphQueryTimout: 5 * time.Second,
	}
	limits := graph.Limits{MaxDepth: cfg.GraphMaxDepth, MaxPaths: cfg.GraphMaxPaths, MaxNodes: cfg.GraphMaxNodes}
	store := eventstore.NewMemoryStore()

	g1 := graph.New(limits)
	svc1 := service.New(repository.NewMemoryGraphAdapter(g1), store, cfg)
	h := NewHandler(svc1).Router(nil, 10*time.Second)

	a := createNode(t, h, "应用A", "application", map[string]any{"owner": "张三"})
	b := createNode(t, h, "数据库B", "database", nil)
	createEdge(t, h, a, b, "reads_from", 2.5)
	call(t, h, http.MethodPut, "/api/v1/nodes/"+a, map[string]any{
		"properties": map[string]any{"risk_level": "high"},
	})

	// 模拟进程重启：换一张空图，只靠事件日志重建。
	g2 := graph.New(limits)
	svc2 := service.New(repository.NewMemoryGraphAdapter(g2), store, cfg)
	if _, err := svc2.ReplayFromLog(t.Context()); err != nil {
		t.Fatalf("重启重放失败: %v", err)
	}

	if g1.NodeCount() != g2.NodeCount() || g1.EdgeCount() != g2.EdgeCount() {
		t.Fatalf("重启后规模不一致: %d/%d 节点，%d/%d 边",
			g1.NodeCount(), g2.NodeCount(), g1.EdgeCount(), g2.EdgeCount())
	}
	n1, _ := g1.GetNode(a)
	n2, _ := g2.GetNode(a)
	if n1.Properties["risk_level"] != n2.Properties["risk_level"] {
		t.Errorf("重启后属性不一致: %v vs %v", n1.Properties, n2.Properties)
	}
	e1, e2 := g1.Edges(), g2.Edges()
	if len(e1) != len(e2) || (len(e1) > 0 && e1[0].Weight != e2[0].Weight) {
		t.Error("重启后关系权重不一致")
	}
}

package graph

import (
	"errors"
	"testing"
)

func TestLineageUpstreamDownstream(t *testing.T) {
	g := mustGraph(t)

	res, err := g.Lineage(t.Context(), LineageOptions{Root: "svc-user"})
	if err != nil {
		t.Fatalf("Lineage 失败: %v", err)
	}

	// svc-user 的上游是调用方 api-gw 与 app-web；下游是它依赖的 db-user 与 srv-01。
	wantUp := map[NodeID]bool{"api-gw": true, "app-web": true}
	wantDown := map[NodeID]bool{"db-user": true, "srv-01": true}

	if len(res.Upstream) != len(wantUp) {
		t.Fatalf("期望 %d 个上游，实际 %d: %v", len(wantUp), len(res.Upstream), nodeIDs(res.Upstream))
	}
	for _, n := range res.Upstream {
		if !wantUp[n.ID] {
			t.Errorf("意外的上游节点 %s", n.ID)
		}
	}
	if len(res.Downstream) != len(wantDown) {
		t.Fatalf("期望 %d 个下游，实际 %d: %v", len(wantDown), len(res.Downstream), nodeIDs(res.Downstream))
	}
	for _, n := range res.Downstream {
		if !wantDown[n.ID] {
			t.Errorf("意外的下游节点 %s", n.ID)
		}
	}
}

// TestLineageSignedLevels 层级用符号区分上下游，供前端分层布局把上游排在上方。
func TestLineageSignedLevels(t *testing.T) {
	g := mustGraph(t)
	res, err := g.Lineage(t.Context(), LineageOptions{Root: "svc-user"})
	if err != nil {
		t.Fatalf("Lineage 失败: %v", err)
	}

	want := map[NodeID]int{
		"svc-user": 0,
		"api-gw":   -1,
		"app-web":  -1, // app-web 直连 svc-user，最近距离为 1 跳
		"db-user":  1,
		"srv-01":   1,
	}
	for id, lvl := range want {
		if got, ok := res.Levels[id]; !ok {
			t.Errorf("层级表缺少节点 %s", id)
		} else if got != lvl {
			t.Errorf("节点 %s 期望层级 %d，实际 %d", id, lvl, got)
		}
	}
}

func TestLineageRootNotFound(t *testing.T) {
	g := mustGraph(t)
	if _, err := g.Lineage(t.Context(), LineageOptions{Root: "ghost"}); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("期望 ErrNodeNotFound，实际: %v", err)
	}
}

func TestLineageLeafNode(t *testing.T) {
	g := mustGraph(t)
	res, err := g.Lineage(t.Context(), LineageOptions{Root: "db-order"})
	if err != nil {
		t.Fatalf("Lineage 失败: %v", err)
	}
	if res.DownstreamCount != 0 {
		t.Errorf("db-order 无出边，下游应为 0，实际 %d", res.DownstreamCount)
	}
	if res.UpstreamCount == 0 {
		t.Error("db-order 应有上游调用方")
	}
}

func TestImpactSummary(t *testing.T) {
	g := mustGraph(t)

	s, err := g.Impact(t.Context(), "api-gw", 0)
	if err != nil {
		t.Fatalf("Impact 失败: %v", err)
	}
	if s.DirectDownstrem != 2 {
		t.Errorf("api-gw 直接下游期望 2（svc-user、svc-order），实际 %d", s.DirectDownstrem)
	}
	// 全部下游：svc-user、svc-order、db-user、srv-01、db-order。
	if s.TotalDownstream != 5 {
		t.Errorf("api-gw 全部下游期望 5，实际 %d", s.TotalDownstream)
	}
	if s.ByType[NodeTypeDatabase] != 2 {
		t.Errorf("下游数据库期望 2 个，实际 %d", s.ByType[NodeTypeDatabase])
	}
	if s.DirectUpstream != 1 {
		t.Errorf("api-gw 直接上游期望 1（app-web），实际 %d", s.DirectUpstream)
	}
}

func TestRootsAndLeaves(t *testing.T) {
	g := mustGraph(t)
	roots, leaves := g.RootsAndLeaves()

	if len(roots) != 1 || roots[0].ID != "app-web" {
		t.Errorf("期望唯一源头 app-web，实际 %v", nodeIDs(roots))
	}
	wantLeaves := map[NodeID]bool{"db-user": true, "db-order": true, "srv-01": true}
	if len(leaves) != len(wantLeaves) {
		t.Fatalf("期望 %d 个末端节点，实际 %d: %v", len(wantLeaves), len(leaves), nodeIDs(leaves))
	}
	for _, n := range leaves {
		if !wantLeaves[n.ID] {
			t.Errorf("意外的末端节点 %s", n.ID)
		}
	}
}

func TestTopologicalOrder(t *testing.T) {
	g := mustGraph(t)

	order, err := g.TopologicalOrder()
	if err != nil {
		t.Fatalf("DAG 应能给出完整拓扑序: %v", err)
	}
	if len(order) != g.NodeCount() {
		t.Fatalf("拓扑序应覆盖全部 %d 个节点，实际 %d", g.NodeCount(), len(order))
	}

	// 拓扑序的定义：每条有向边的起点必须排在终点之前。
	pos := make(map[NodeID]int, len(order))
	for i, n := range order {
		pos[n.ID] = i
	}
	for _, e := range g.Edges() {
		if !e.Directed {
			continue
		}
		if pos[e.Source] > pos[e.Target] {
			t.Errorf("边 %s: 起点 %s(位置 %d) 应排在终点 %s(位置 %d) 之前",
				e.ID, e.Source, pos[e.Source], e.Target, pos[e.Target])
		}
	}
}

func TestTopologicalOrderDetectsCycle(t *testing.T) {
	g := New(DefaultLimits())
	for _, id := range []string{"a", "b"} {
		if err := g.AddNode(newNode(id, "节点"+id, NodeTypeApplication, nil)); err != nil {
			t.Fatal(err)
		}
	}
	_ = g.AddEdge(newEdge("ab", "a", "b", RelCalls, 1))
	_ = g.AddEdge(newEdge("ba", "b", "a", RelDependsOn, 1))

	partial, err := g.TopologicalOrder()
	if err == nil {
		t.Fatal("存在循环依赖时应返回错误")
	}
	if len(partial) != 0 {
		t.Errorf("完全成环时不应排出任何节点，实际 %v", nodeIDs(partial))
	}
}

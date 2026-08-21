package graph

import (
	"fmt"
	"testing"
)

func TestAllPathsEnumeration(t *testing.T) {
	g := mustGraph(t)

	res, err := g.AllPaths(t.Context(), AllPathsOptions{
		From: "app-web", To: "svc-user", Direction: DirectionOut,
	})
	if err != nil {
		t.Fatalf("AllPaths 失败: %v", err)
	}

	// app-web→svc-user 恰有两条简单路径：直连（代价 4）与经 api-gw（代价 3）。
	if len(res.Paths) != 2 {
		t.Fatalf("期望 2 条路径，实际 %d: %v", len(res.Paths), pathSummaries(res.Paths))
	}
	// 结果按代价升序，低代价路径应排在前面。
	if res.Paths[0].TotalCost != 3 || res.Paths[1].TotalCost != 4 {
		t.Errorf("期望代价序列 [3 4]，实际 %v", pathSummaries(res.Paths))
	}
	if res.Paths[0].Hops != 2 || res.Paths[1].Hops != 1 {
		t.Errorf("期望跳数序列 [2 1]，实际 %v", pathSummaries(res.Paths))
	}
}

func TestAllPathsNoRepeatedNodes(t *testing.T) {
	g := New(DefaultLimits())
	for _, id := range []string{"a", "b", "c", "d"} {
		if err := g.AddNode(newNode(id, "节点"+id, NodeTypeApplication, nil)); err != nil {
			t.Fatal(err)
		}
	}
	// 构造含环图：a→b→c→a，另有 c→d。
	_ = g.AddEdge(newEdge("ab", "a", "b", RelCalls, 1))
	_ = g.AddEdge(newEdge("bc", "b", "c", RelCalls, 1))
	_ = g.AddEdge(newEdge("ca", "c", "a", RelCalls, 1))
	_ = g.AddEdge(newEdge("cd", "c", "d", RelCalls, 1))

	res, err := g.AllPaths(t.Context(), AllPathsOptions{From: "a", To: "d", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("AllPaths 失败: %v", err)
	}
	if len(res.Paths) != 1 {
		t.Fatalf("环图中 a→d 只有一条简单路径，实际 %d 条", len(res.Paths))
	}
	// 简单路径不得重复经过同一节点，否则环会导致无限枚举。
	for _, p := range res.Paths {
		seen := map[NodeID]bool{}
		for _, n := range p.Nodes {
			if seen[n.ID] {
				t.Fatalf("路径 %v 重复经过节点 %s", nodeIDs(p.Nodes), n.ID)
			}
			seen[n.ID] = true
		}
	}
}

func TestAllPathsMaxPathsLimit(t *testing.T) {
	// 构造一张路径数呈组合爆炸的图：起点与终点之间有 8 个并联的中间节点，
	// 两两之间全连接，简单路径数量远超上限。
	g := New(Limits{MaxDepth: 10, MaxPaths: 5, MaxNodes: 1000})
	if err := g.AddNode(newNode("src", "起点", NodeTypeApplication, nil)); err != nil {
		t.Fatal(err)
	}
	if err := g.AddNode(newNode("dst", "终点", NodeTypeApplication, nil)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("m%d", i)
		if err := g.AddNode(newNode(id, "中转"+id, NodeTypeApplication, nil)); err != nil {
			t.Fatal(err)
		}
		_ = g.AddEdge(newEdge("s-"+id, "src", id, RelCalls, 1))
		_ = g.AddEdge(newEdge(id+"-d", id, "dst", RelCalls, 1))
	}
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			if i != j {
				_ = g.AddEdge(newEdge(fmt.Sprintf("m%d-m%d", i, j), fmt.Sprintf("m%d", i), fmt.Sprintf("m%d", j), RelCalls, 1))
			}
		}
	}

	res, err := g.AllPaths(t.Context(), AllPathsOptions{From: "src", To: "dst", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("AllPaths 失败: %v", err)
	}
	if !res.Truncated {
		t.Error("路径数超过上限时应标记 Truncated")
	}
	if len(res.Paths) > 5 {
		t.Errorf("返回路径数 %d 超过上限 5", len(res.Paths))
	}
}

func TestAllPathsMaxDepthLimit(t *testing.T) {
	g := mustGraph(t)
	res, err := g.AllPaths(t.Context(), AllPathsOptions{
		From: "app-web", To: "db-order", Direction: DirectionOut, MaxDepth: 2,
	})
	if err != nil {
		t.Fatalf("AllPaths 失败: %v", err)
	}
	// app-web→api-gw→svc-order→db-order 需要 3 跳，深度限制 2 时应无解。
	if len(res.Paths) != 0 {
		t.Errorf("深度限制 2 时不应找到路径，实际 %v", pathSummaries(res.Paths))
	}
}

func TestAllPathsSameNode(t *testing.T) {
	g := mustGraph(t)
	res, err := g.AllPaths(t.Context(), AllPathsOptions{From: "app-web", To: "app-web"})
	if err != nil {
		t.Fatalf("AllPaths 失败: %v", err)
	}
	if len(res.Paths) != 1 || res.Paths[0].Hops != 0 {
		t.Errorf("起点即终点应返回一条零跳路径，实际 %v", pathSummaries(res.Paths))
	}
}

func TestAllPathsUnion(t *testing.T) {
	g := mustGraph(t)
	res, err := g.AllPaths(t.Context(), AllPathsOptions{From: "app-web", To: "svc-user", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("AllPaths 失败: %v", err)
	}
	// 两条路径的并集应为 {app-web, api-gw, svc-user} 与 3 条边。
	if len(res.Nodes) != 3 {
		t.Errorf("期望并集 3 个节点，实际 %d: %v", len(res.Nodes), nodeIDs(res.Nodes))
	}
	if len(res.Edges) != 3 {
		t.Errorf("期望并集 3 条边，实际 %d: %v", len(res.Edges), edgeIDs(res.Edges))
	}
}

func TestAllPathsNodeNotFound(t *testing.T) {
	g := mustGraph(t)
	if _, err := g.AllPaths(t.Context(), AllPathsOptions{From: "ghost", To: "app-web"}); err == nil {
		t.Fatal("起点不存在时应报错")
	}
}

func pathSummaries(ps []*PathResult) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = fmt.Sprintf("{cost:%v hops:%d path:%v}", p.TotalCost, p.Hops, nodeIDs(p.Nodes))
	}
	return out
}

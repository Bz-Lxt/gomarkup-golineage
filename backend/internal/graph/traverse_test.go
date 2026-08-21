package graph

import (
	"context"
	"testing"
)

func TestBFSDepths(t *testing.T) {
	g := mustGraph(t)

	res, err := g.BFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("BFS 失败: %v", err)
	}

	// app-web 有两条路到 svc-user：直连（1 跳）与经 api-gw（2 跳）。
	// BFS 记录的必须是最少跳数。
	want := map[NodeID]int{
		"app-web": 0, "api-gw": 1, "svc-user": 1,
		"svc-order": 2, "db-user": 2, "srv-01": 2, "db-order": 3,
	}
	for id, d := range want {
		if got, ok := res.Depths[id]; !ok {
			t.Errorf("节点 %s 未被访问", id)
		} else if got != d {
			t.Errorf("节点 %s 期望深度 %d，实际 %d", id, d, got)
		}
	}
	if res.VisitedCount != 7 {
		t.Errorf("期望访问 7 个节点，实际 %d", res.VisitedCount)
	}
	if res.Algorithm != "bfs" {
		t.Errorf("期望 algorithm=bfs，实际 %s", res.Algorithm)
	}
}

func TestBFSMaxDepth(t *testing.T) {
	g := mustGraph(t)

	res, err := g.BFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionOut, MaxDepth: 1})
	if err != nil {
		t.Fatalf("BFS 失败: %v", err)
	}
	if res.VisitedCount != 3 {
		t.Fatalf("深度 1 期望访问 3 个节点（自身+2 邻居），实际 %d: %v", res.VisitedCount, nodeIDs(res.Nodes))
	}
	if _, ok := res.Depths["db-user"]; ok {
		t.Error("深度 1 不应触达 db-user")
	}
}

func TestBFSDirection(t *testing.T) {
	g := mustGraph(t)

	t.Run("入边方向追溯上游", func(t *testing.T) {
		res, err := g.BFS(t.Context(), TraverseOptions{Start: "db-user", Direction: DirectionIn})
		if err != nil {
			t.Fatalf("BFS 失败: %v", err)
		}
		want := []NodeID{"db-user", "svc-user", "api-gw", "app-web"}
		for _, id := range want {
			if _, ok := res.Depths[id]; !ok {
				t.Errorf("上游应包含 %s，实际: %v", id, nodeIDs(res.Nodes))
			}
		}
		if _, ok := res.Depths["srv-01"]; ok {
			t.Error("srv-01 是 svc-user 的下游，不应出现在 db-user 的上游中")
		}
	})

	t.Run("出边方向无上游", func(t *testing.T) {
		res, _ := g.BFS(t.Context(), TraverseOptions{Start: "db-user", Direction: DirectionOut})
		if res.VisitedCount != 1 {
			t.Errorf("db-user 无出边，期望只访问自身，实际 %d", res.VisitedCount)
		}
	})

	t.Run("双向应覆盖全图", func(t *testing.T) {
		res, _ := g.BFS(t.Context(), TraverseOptions{Start: "db-user", Direction: DirectionBoth})
		if res.VisitedCount != 7 {
			t.Errorf("双向遍历期望覆盖 7 个节点，实际 %d", res.VisitedCount)
		}
	})
}

func TestBFSRelationFilter(t *testing.T) {
	g := mustGraph(t)

	res, err := g.BFS(t.Context(), TraverseOptions{
		Start:     "app-web",
		Direction: DirectionOut,
		Relations: []RelationType{RelCalls},
	})
	if err != nil {
		t.Fatalf("BFS 失败: %v", err)
	}
	// 只沿 calls 前进：db-user / db-order 经 reads_from 连接，srv-01 经 deploys_on 连接，均不可达。
	for _, id := range []NodeID{"db-user", "db-order", "srv-01"} {
		if _, ok := res.Depths[id]; ok {
			t.Errorf("关系过滤为 calls 时不应触达 %s", id)
		}
	}
	if res.VisitedCount != 4 {
		t.Errorf("期望访问 4 个节点，实际 %d: %v", res.VisitedCount, nodeIDs(res.Nodes))
	}
}

func TestBFSNodeTypeFilter(t *testing.T) {
	g := mustGraph(t)

	res, _ := g.BFS(t.Context(), TraverseOptions{
		Start:     "app-web",
		Direction: DirectionOut,
		NodeTypes: []NodeType{NodeTypeApplication, NodeTypeAPI},
	})
	for _, n := range res.Nodes {
		if n.Type != NodeTypeApplication && n.Type != NodeTypeAPI {
			t.Errorf("类型过滤失效，出现了 %s 类型的节点 %s", n.Type, n.ID)
		}
	}
}

func TestBFSStartNotFound(t *testing.T) {
	g := mustGraph(t)
	if _, err := g.BFS(t.Context(), TraverseOptions{Start: "ghost"}); err == nil {
		t.Fatal("起点不存在时应返回错误")
	}
}

func TestBFSTruncation(t *testing.T) {
	g := mustGraph(t)
	res, err := g.BFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionBoth, MaxNodes: 3})
	if err != nil {
		t.Fatalf("BFS 失败: %v", err)
	}
	if !res.Truncated {
		t.Fatal("超过 MaxNodes 时应标记 Truncated")
	}
	if res.VisitedCount > 3 {
		t.Errorf("访问数 %d 超过上限 3", res.VisitedCount)
	}
}

func TestBFSRespectsContextCancel(t *testing.T) {
	g := mustGraph(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := g.BFS(ctx, TraverseOptions{Start: "app-web"}); err == nil {
		t.Fatal("已取消的 context 应中止遍历")
	}
}

func TestBFSInducedEdges(t *testing.T) {
	g := mustGraph(t)

	res, _ := g.BFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionOut, MaxDepth: 1})
	// 点集为 {app-web, api-gw, svc-user}，其内部边应有 e1(app-web→api-gw)、
	// e4(app-web→svc-user)、e2(api-gw→svc-user) 三条 —— e2 是横叉边，
	// 只返回遍历树边会漏掉它，画布上就会缺一条真实存在的调用关系。
	got := map[EdgeID]bool{}
	for _, e := range res.Edges {
		got[e.ID] = true
	}
	for _, want := range []EdgeID{"e1", "e2", "e4"} {
		if !got[want] {
			t.Errorf("诱导子图应包含边 %s，实际: %v", want, edgeIDs(res.Edges))
		}
	}
	if len(res.Edges) != 3 {
		t.Errorf("期望 3 条诱导边，实际 %d: %v", len(res.Edges), edgeIDs(res.Edges))
	}
}

func TestDFSReachesSameNodes(t *testing.T) {
	g := mustGraph(t)

	bfs, _ := g.BFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionOut})
	dfs, err := g.DFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("DFS 失败: %v", err)
	}

	// 遍历顺序不同，但可达点集必须一致。
	if dfs.VisitedCount != bfs.VisitedCount {
		t.Fatalf("DFS 访问 %d 个节点，BFS 访问 %d 个，可达集应一致", dfs.VisitedCount, bfs.VisitedCount)
	}
	if dfs.Algorithm != "dfs" {
		t.Errorf("期望 algorithm=dfs，实际 %s", dfs.Algorithm)
	}
	if dfs.CycleDetected {
		t.Error("测试图为 DAG，不应报告存在环")
	}
}

// TestDFSCycleDetection 有向环必须被识别。
// 循环依赖在血缘图中是真实存在的故障模式，静默忽略会让用户误判拓扑。
func TestDFSCycleDetection(t *testing.T) {
	g := New(DefaultLimits())
	for _, id := range []string{"a", "b", "c"} {
		if err := g.AddNode(newNode(id, "节点"+id, NodeTypeApplication, nil)); err != nil {
			t.Fatal(err)
		}
	}
	_ = g.AddEdge(newEdge("ab", "a", "b", RelCalls, 1))
	_ = g.AddEdge(newEdge("bc", "b", "c", RelCalls, 1))
	_ = g.AddEdge(newEdge("ca", "c", "a", RelCalls, 1))

	res, err := g.DFS(t.Context(), TraverseOptions{Start: "a", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("DFS 失败: %v", err)
	}
	if !res.CycleDetected {
		t.Fatal("a→b→c→a 构成环，应被检测到")
	}
	if res.VisitedCount != 3 {
		t.Errorf("环图应访问 3 个节点且不死循环，实际 %d", res.VisitedCount)
	}
}

func TestDFSMaxDepth(t *testing.T) {
	g := mustGraph(t)
	res, err := g.DFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionOut, MaxDepth: 1})
	if err != nil {
		t.Fatalf("DFS 失败: %v", err)
	}
	for id, d := range res.Depths {
		if d > 1 {
			t.Errorf("节点 %s 深度 %d 超过限制 1", id, d)
		}
	}
}

func TestNeighborSubgraph(t *testing.T) {
	g := mustGraph(t)

	res, err := g.NeighborSubgraph(t.Context(), "svc-user", 1, nil)
	if err != nil {
		t.Fatalf("NeighborSubgraph 失败: %v", err)
	}
	// svc-user 的一跳邻居：上游 api-gw、app-web，下游 db-user、srv-01，加自身共 5 个。
	if res.VisitedCount != 5 {
		t.Fatalf("期望 5 个节点，实际 %d: %v", res.VisitedCount, nodeIDs(res.Nodes))
	}
	for _, id := range []NodeID{"api-gw", "app-web", "db-user", "srv-01"} {
		if _, ok := res.Depths[id]; !ok {
			t.Errorf("一跳邻居应包含 %s", id)
		}
	}
}

func TestNeighborSubgraphDefaultsToOneHop(t *testing.T) {
	g := mustGraph(t)
	res, err := g.NeighborSubgraph(t.Context(), "svc-user", 0, nil)
	if err != nil {
		t.Fatalf("NeighborSubgraph 失败: %v", err)
	}
	for id, d := range res.Depths {
		if d > 1 {
			t.Errorf("hops<=0 应退化为 1 跳，节点 %s 深度为 %d", id, d)
		}
	}
}

func TestDeterministicOrder(t *testing.T) {
	g := mustGraph(t)

	// map 迭代顺序随机，若邻接结构直接用 map，多次遍历结果会不同，
	// 导致测试无法断言、前端布局每次刷新都在跳动。
	first, _ := g.BFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionBoth})
	for i := 0; i < 20; i++ {
		again, _ := g.BFS(t.Context(), TraverseOptions{Start: "app-web", Direction: DirectionBoth})
		if len(again.Order) != len(first.Order) {
			t.Fatalf("第 %d 次遍历长度不一致", i)
		}
		for j := range first.Order {
			if first.Order[j] != again.Order[j] {
				t.Fatalf("第 %d 次遍历顺序发生漂移:\n首次 %v\n本次 %v", i, first.Order, again.Order)
			}
		}
	}
}

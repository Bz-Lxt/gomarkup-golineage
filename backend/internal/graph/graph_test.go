package graph

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// newNode 构造测试用节点。
func newNode(id, name string, t NodeType, props Properties) *Node {
	return &Node{ID: id, Name: name, Type: t, Properties: props}
}

// newEdge 构造测试用有向边。
func newEdge(id, src, dst string, rel RelationType, w float64) *Edge {
	return &Edge{ID: id, Source: src, Target: dst, Relation: rel, Weight: w, Directed: true}
}

// mustGraph 构建一张固定的 IT 资产测试图，各用例共用以便断言可预期的结果。
//
//	app-web --calls(1)--> api-gw --calls(2)--> svc-user --reads_from(1)--> db-user
//	app-web --calls(4)--> svc-user
//	svc-user --deploys_on(1)--> srv-01
//	api-gw --calls(3)--> svc-order --reads_from(1)--> db-order
func mustGraph(t *testing.T) *Graph {
	t.Helper()
	g := New(DefaultLimits())

	nodes := []*Node{
		newNode("app-web", "门户应用", NodeTypeApplication, Properties{"owner": "张三", "risk_level": "high"}),
		newNode("api-gw", "API 网关", NodeTypeAPI, Properties{"ip": "10.0.0.1"}),
		newNode("svc-user", "用户服务", NodeTypeApplication, Properties{"owner": "李四"}),
		newNode("svc-order", "订单服务", NodeTypeApplication, nil),
		newNode("db-user", "用户库", NodeTypeDatabase, Properties{"risk_level": "high"}),
		newNode("db-order", "订单库", NodeTypeDatabase, nil),
		newNode("srv-01", "宿主机 01", NodeTypeServer, Properties{"ip": "10.0.0.9"}),
	}
	for _, n := range nodes {
		if err := g.AddNode(n); err != nil {
			t.Fatalf("AddNode(%s) 失败: %v", n.ID, err)
		}
	}

	edges := []*Edge{
		newEdge("e1", "app-web", "api-gw", RelCalls, 1),
		newEdge("e2", "api-gw", "svc-user", RelCalls, 2),
		newEdge("e3", "svc-user", "db-user", RelReadsFrom, 1),
		newEdge("e4", "app-web", "svc-user", RelCalls, 4),
		newEdge("e5", "svc-user", "srv-01", RelDeploysOn, 1),
		newEdge("e6", "api-gw", "svc-order", RelCalls, 3),
		newEdge("e7", "svc-order", "db-order", RelReadsFrom, 1),
	}
	for _, e := range edges {
		if err := g.AddEdge(e); err != nil {
			t.Fatalf("AddEdge(%s) 失败: %v", e.ID, err)
		}
	}
	return g
}

func TestAddNode(t *testing.T) {
	t.Run("正常新增", func(t *testing.T) {
		g := New(DefaultLimits())
		if err := g.AddNode(newNode("n1", "服务器A", NodeTypeServer, nil)); err != nil {
			t.Fatalf("期望成功，实际: %v", err)
		}
		if g.NodeCount() != 1 {
			t.Fatalf("期望节点数 1，实际 %d", g.NodeCount())
		}
	})

	t.Run("重复 ID 应拒绝", func(t *testing.T) {
		g := New(DefaultLimits())
		_ = g.AddNode(newNode("n1", "A", NodeTypeServer, nil))
		err := g.AddNode(newNode("n1", "B", NodeTypeServer, nil))
		if !errors.Is(err, ErrNodeExists) {
			t.Fatalf("期望 ErrNodeExists，实际: %v", err)
		}
	})

	t.Run("非法类型应拒绝", func(t *testing.T) {
		g := New(DefaultLimits())
		err := g.AddNode(newNode("n1", "A", NodeType("unknown"), nil))
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("期望 ErrValidation，实际: %v", err)
		}
	})

	t.Run("空名称应拒绝", func(t *testing.T) {
		g := New(DefaultLimits())
		if err := g.AddNode(newNode("n1", "   ", NodeTypeServer, nil)); !errors.Is(err, ErrValidation) {
			t.Fatalf("期望 ErrValidation，实际: %v", err)
		}
	})

	t.Run("非法属性键应拒绝", func(t *testing.T) {
		g := New(DefaultLimits())
		err := g.AddNode(newNode("n1", "A", NodeTypeServer, Properties{"bad key!": 1}))
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("期望 ErrValidation，实际: %v", err)
		}
	})

	t.Run("中文属性键应接受", func(t *testing.T) {
		g := New(DefaultLimits())
		if err := g.AddNode(newNode("n1", "A", NodeTypeServer, Properties{"责任人": "王五"})); err != nil {
			t.Fatalf("期望成功，实际: %v", err)
		}
	})

	t.Run("属性数量超限应拒绝", func(t *testing.T) {
		g := New(DefaultLimits())
		props := make(Properties, MaxProperties+1)
		for i := 0; i <= MaxProperties; i++ {
			props[fmt.Sprintf("k%d", i)] = i
		}
		if err := g.AddNode(newNode("n1", "A", NodeTypeServer, props)); !errors.Is(err, ErrValidation) {
			t.Fatalf("期望 ErrValidation，实际: %v", err)
		}
	})
}

// TestGetNodeReturnsCopy 验证对外返回的是深拷贝。
// 若返回内部指针，调用方的修改会绕过读写锁直接污染图状态。
func TestGetNodeReturnsCopy(t *testing.T) {
	g := mustGraph(t)

	got, err := g.GetNode("app-web")
	if err != nil {
		t.Fatalf("GetNode 失败: %v", err)
	}
	got.Name = "被篡改"
	got.Properties["owner"] = "入侵者"

	again, _ := g.GetNode("app-web")
	if again.Name != "门户应用" {
		t.Errorf("名称被外部修改污染: %s", again.Name)
	}
	if again.Properties["owner"] != "张三" {
		t.Errorf("属性被外部修改污染: %v", again.Properties["owner"])
	}
}

func TestAddEdge(t *testing.T) {
	t.Run("端点不存在应拒绝", func(t *testing.T) {
		g := New(DefaultLimits())
		_ = g.AddNode(newNode("a", "A", NodeTypeServer, nil))
		err := g.AddEdge(newEdge("e1", "a", "ghost", RelCalls, 1))
		if !errors.Is(err, ErrNodeNotFound) {
			t.Fatalf("期望 ErrNodeNotFound，实际: %v", err)
		}
	})

	t.Run("自环应拒绝", func(t *testing.T) {
		g := New(DefaultLimits())
		_ = g.AddNode(newNode("a", "A", NodeTypeServer, nil))
		if err := g.AddEdge(newEdge("e1", "a", "a", RelCalls, 1)); !errors.Is(err, ErrValidation) {
			t.Fatalf("期望 ErrValidation，实际: %v", err)
		}
	})

	t.Run("重复边应拒绝", func(t *testing.T) {
		g := mustGraph(t)
		err := g.AddEdge(newEdge("e-dup", "app-web", "api-gw", RelCalls, 9))
		if !errors.Is(err, ErrEdgeExists) {
			t.Fatalf("期望 ErrEdgeExists，实际: %v", err)
		}
	})

	t.Run("同端点不同关系类型应允许", func(t *testing.T) {
		g := mustGraph(t)
		if err := g.AddEdge(newEdge("e-new", "app-web", "api-gw", RelDependsOn, 1)); err != nil {
			t.Fatalf("期望成功，实际: %v", err)
		}
	})

	t.Run("非正权重应拒绝", func(t *testing.T) {
		g := mustGraph(t)
		for _, w := range []float64{0, -1} {
			err := g.AddEdge(newEdge("e-bad", "db-user", "srv-01", RelDependsOn, w))
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("权重 %v 期望 ErrValidation，实际: %v", w, err)
			}
		}
	})
}

// TestRemoveNodeCascade 删除节点必须级联清理关联边并返回它们，
// 否则历史回溯会重建出指向已删除节点的悬空边。
func TestRemoveNodeCascade(t *testing.T) {
	g := mustGraph(t)

	removed, err := g.RemoveNode("svc-user")
	if err != nil {
		t.Fatalf("RemoveNode 失败: %v", err)
	}

	wantEdges := map[EdgeID]bool{"e2": true, "e3": true, "e4": true, "e5": true}
	if len(removed) != len(wantEdges) {
		t.Fatalf("期望级联删除 %d 条边，实际 %d 条: %v", len(wantEdges), len(removed), edgeIDs(removed))
	}
	for _, e := range removed {
		if !wantEdges[e.ID] {
			t.Errorf("意外删除了边 %s", e.ID)
		}
	}

	if g.NodeCount() != 6 {
		t.Errorf("期望剩余 6 个节点，实际 %d", g.NodeCount())
	}
	if g.EdgeCount() != 3 {
		t.Errorf("期望剩余 3 条边，实际 %d", g.EdgeCount())
	}

	// 邻接表必须同步清理，不能残留已删除边的 ID。
	for _, id := range []NodeID{"app-web", "api-gw", "db-user", "srv-01"} {
		d, _ := g.Degree(id, DirectionBoth)
		neighbors, _ := g.NeighborSubgraph(t.Context(), id, 1, nil)
		for _, e := range neighbors.Edges {
			if wantEdges[e.ID] {
				t.Errorf("节点 %s 的邻接表仍残留已删除边 %s（度数 %d）", id, e.ID, d)
			}
		}
	}
}

func TestRemoveNodeNotFound(t *testing.T) {
	g := New(DefaultLimits())
	if _, err := g.RemoveNode("ghost"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("期望 ErrNodeNotFound，实际: %v", err)
	}
}

func TestUpdateEdge(t *testing.T) {
	t.Run("修改端点应拒绝", func(t *testing.T) {
		g := mustGraph(t)
		e, _ := g.GetEdge("e1")
		e.Target = "db-user"
		if err := g.UpdateEdge(e); !errors.Is(err, ErrValidation) {
			t.Fatalf("期望 ErrValidation，实际: %v", err)
		}
	})

	t.Run("修改权重应生效", func(t *testing.T) {
		g := mustGraph(t)
		e, _ := g.GetEdge("e1")
		e.Weight = 99
		if err := g.UpdateEdge(e); err != nil {
			t.Fatalf("期望成功，实际: %v", err)
		}
		got, _ := g.GetEdge("e1")
		if got.Weight != 99 {
			t.Errorf("期望权重 99，实际 %v", got.Weight)
		}
	})

	t.Run("改成已存在的关系应拒绝", func(t *testing.T) {
		g := mustGraph(t)
		_ = g.AddEdge(newEdge("e-extra", "app-web", "api-gw", RelDependsOn, 1))
		e, _ := g.GetEdge("e-extra")
		e.Relation = RelCalls // 与 e1 撞键
		if err := g.UpdateEdge(e); !errors.Is(err, ErrEdgeExists) {
			t.Fatalf("期望 ErrEdgeExists，实际: %v", err)
		}
	})
}

// TestUndirectedEdge 无向边在两个方向上都应可达。
func TestUndirectedEdge(t *testing.T) {
	g := New(DefaultLimits())
	_ = g.AddNode(newNode("p1", "张三", NodeTypePerson, nil))
	_ = g.AddNode(newNode("p2", "李四", NodeTypePerson, nil))

	e := &Edge{ID: "u1", Source: "p1", Target: "p2", Relation: RelAssociatesWith, Weight: 1, Directed: false}
	if err := g.AddEdge(e); err != nil {
		t.Fatalf("AddEdge 失败: %v", err)
	}

	for _, tc := range []struct{ from, want NodeID }{{"p1", "p2"}, {"p2", "p1"}} {
		res, err := g.BFS(t.Context(), TraverseOptions{Start: tc.from, Direction: DirectionOut, MaxDepth: 1})
		if err != nil {
			t.Fatalf("BFS(%s) 失败: %v", tc.from, err)
		}
		if _, ok := res.Depths[tc.want]; !ok {
			t.Errorf("从 %s 沿出边应能到达 %s，实际深度表: %v", tc.from, tc.want, res.Depths)
		}
	}

	// 反向重复的无向边应被去重键拦截。
	dup := &Edge{ID: "u2", Source: "p2", Target: "p1", Relation: RelAssociatesWith, Weight: 1, Directed: false}
	if err := g.AddEdge(dup); !errors.Is(err, ErrEdgeExists) {
		t.Errorf("无向边 A-B 与 B-A 应视为重复，实际: %v", err)
	}
}

// TestConcurrentReadWrite 在 -race 下验证读写并发安全。
func TestConcurrentReadWrite(t *testing.T) {
	g := mustGraph(t)
	const workers = 16

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				switch i % 4 {
				case 0:
					_, _ = g.GetNode("app-web")
				case 1:
					_, _ = g.BFS(t.Context(), TraverseOptions{Start: "app-web", MaxDepth: 3, Direction: DirectionBoth})
				case 2:
					_, _ = g.ShortestPath(t.Context(), PathOptions{From: "app-web", To: "db-user", Direction: DirectionOut})
				case 3:
					_ = g.Stats()
				}
			}
		}(i)
	}

	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("tmp-%d", i)
		_ = g.AddNode(newNode(id, "临时"+id, NodeTypeStorage, Properties{"seq": i}))
		_ = g.AddEdge(newEdge("te-"+id, "srv-01", id, RelDependsOn, 1))
		if i%3 == 0 {
			_, _ = g.RemoveNode(id)
		}
	}

	close(stop)
	wg.Wait()

	if g.NodeCount() == 0 {
		t.Fatal("并发操作后图不应为空")
	}
}

func TestStats(t *testing.T) {
	g := mustGraph(t)
	s := g.Stats()

	if s.NodeCount != 7 {
		t.Errorf("期望 7 个节点，实际 %d", s.NodeCount)
	}
	if s.EdgeCount != 7 {
		t.Errorf("期望 7 条边，实际 %d", s.EdgeCount)
	}
	if s.TypeCounts[NodeTypeApplication] != 3 {
		t.Errorf("期望 3 个 application，实际 %d", s.TypeCounts[NodeTypeApplication])
	}
	if s.Isolated != 0 {
		t.Errorf("测试图不应有孤立节点，实际 %d", s.Isolated)
	}
}

func TestSearch(t *testing.T) {
	g := mustGraph(t)

	t.Run("按类型", func(t *testing.T) {
		got := g.Search(SearchOptions{Types: []NodeType{NodeTypeDatabase}})
		if len(got) != 2 {
			t.Fatalf("期望 2 个数据库节点，实际 %d", len(got))
		}
	})

	t.Run("按属性键值", func(t *testing.T) {
		got := g.Search(SearchOptions{PropKey: "risk_level", PropValue: "high"})
		if len(got) != 2 {
			t.Fatalf("期望 2 个高风险节点，实际 %d: %v", len(got), nodeIDs(got))
		}
	})

	t.Run("按属性键存在", func(t *testing.T) {
		got := g.Search(SearchOptions{PropKey: "ip"})
		if len(got) != 2 {
			t.Fatalf("期望 2 个含 ip 的节点，实际 %d", len(got))
		}
	})

	t.Run("按名称关键字", func(t *testing.T) {
		got := g.Search(SearchOptions{Keyword: "服务"})
		if len(got) != 2 {
			t.Fatalf("期望 2 个含「服务」的节点，实际 %d: %v", len(got), nodeIDs(got))
		}
	})

	t.Run("结果按名称升序", func(t *testing.T) {
		got := g.Search(SearchOptions{})
		for i := 1; i < len(got); i++ {
			if got[i-1].Name > got[i].Name {
				t.Fatalf("结果未按名称升序: %v", nodeIDs(got))
			}
		}
	})

	t.Run("删除后索引应同步", func(t *testing.T) {
		g := mustGraph(t)
		_, _ = g.RemoveNode("db-user")
		got := g.Search(SearchOptions{PropKey: "risk_level", PropValue: "high"})
		if len(got) != 1 {
			t.Fatalf("删除后期望剩 1 个高风险节点，实际 %d: %v", len(got), nodeIDs(got))
		}
	})
}

func TestSnapshotRestore(t *testing.T) {
	g := mustGraph(t)
	snap := g.Snapshot()

	restored := New(DefaultLimits())
	if err := restored.Restore(snap); err != nil {
		t.Fatalf("Restore 失败: %v", err)
	}
	if restored.NodeCount() != g.NodeCount() || restored.EdgeCount() != g.EdgeCount() {
		t.Fatalf("恢复后规模不一致: 节点 %d/%d，边 %d/%d",
			restored.NodeCount(), g.NodeCount(), restored.EdgeCount(), g.EdgeCount())
	}

	// 恢复后的图必须能给出与原图一致的查询结果。
	a, _ := g.ShortestPath(t.Context(), PathOptions{From: "app-web", To: "db-user"})
	b, _ := restored.ShortestPath(t.Context(), PathOptions{From: "app-web", To: "db-user"})
	if a.TotalCost != b.TotalCost || a.Hops != b.Hops {
		t.Errorf("恢复后最短路径不一致: 原 cost=%v hops=%d，新 cost=%v hops=%d",
			a.TotalCost, a.Hops, b.TotalCost, b.Hops)
	}
}

func TestRestoreRejectsDanglingEdge(t *testing.T) {
	g := New(DefaultLimits())
	err := g.Restore(&Snapshot{
		Nodes: []*Node{newNode("a", "A", NodeTypeServer, nil)},
		Edges: []*Edge{newEdge("e1", "a", "ghost", RelCalls, 1)},
	})
	if err == nil {
		t.Fatal("端点缺失的快照应恢复失败，而不是构建出残缺拓扑")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("错误信息应指明缺失端点，实际: %v", err)
	}
}

func TestTopologyTruncation(t *testing.T) {
	g := mustGraph(t)
	topo := g.Topology(TopologyOptions{Limit: 3})

	if !topo.Truncated {
		t.Fatal("超过 Limit 时应标记 Truncated")
	}
	if len(topo.Nodes) != 3 {
		t.Fatalf("期望返回 3 个节点，实际 %d", len(topo.Nodes))
	}
	if topo.TotalNodes != 7 {
		t.Errorf("TotalNodes 应反映过滤后总量 7，实际 %d", topo.TotalNodes)
	}
	// 截断时应保留枢纽节点，否则画布上全是孤点。
	ids := map[NodeID]bool{}
	for _, n := range topo.Nodes {
		ids[n.ID] = true
	}
	if !ids["svc-user"] {
		t.Errorf("度数最高的 svc-user 应被保留，实际保留: %v", nodeIDs(topo.Nodes))
	}
}

func TestInsertRemoveSorted(t *testing.T) {
	var s []EdgeID
	for _, id := range []EdgeID{"c", "a", "b", "a"} {
		s = insertSorted(s, id)
	}
	if got := strings.Join(s, ","); got != "a,b,c" {
		t.Fatalf("期望去重且有序 a,b,c，实际 %s", got)
	}
	s = removeSorted(s, "b")
	if got := strings.Join(s, ","); got != "a,c" {
		t.Fatalf("期望 a,c，实际 %s", got)
	}
	s = removeSorted(s, "zzz")
	if len(s) != 2 {
		t.Fatalf("删除不存在的元素不应改变切片，实际 %v", s)
	}
}

func nodeIDs(ns []*Node) []NodeID {
	out := make([]NodeID, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

func edgeIDs(es []*Edge) []EdgeID {
	out := make([]EdgeID, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}

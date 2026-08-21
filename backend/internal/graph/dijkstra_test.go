package graph

import (
	"context"
	"math/rand"
	"testing"
)

// TestShortestPathKnownAnswer 用一张手工计算过答案的图验证 Dijkstra。
//
//	A --1--> B --2--> D
//	A --4--> C --1--> D
//	B --5--> C
//
// A→D 的候选：A-B-D = 3、A-C-D = 5、A-B-C-D = 7。最优解为 3。
func TestShortestPathKnownAnswer(t *testing.T) {
	g := New(DefaultLimits())
	for _, id := range []string{"A", "B", "C", "D"} {
		if err := g.AddNode(newNode(id, "节点"+id, NodeTypeApplication, nil)); err != nil {
			t.Fatal(err)
		}
	}
	_ = g.AddEdge(newEdge("ab", "A", "B", RelCalls, 1))
	_ = g.AddEdge(newEdge("bd", "B", "D", RelCalls, 2))
	_ = g.AddEdge(newEdge("ac", "A", "C", RelCalls, 4))
	_ = g.AddEdge(newEdge("cd", "C", "D", RelCalls, 1))
	_ = g.AddEdge(newEdge("bc", "B", "C", RelCalls, 5))

	res, err := g.ShortestPath(t.Context(), PathOptions{From: "A", To: "D", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("ShortestPath 失败: %v", err)
	}
	if !res.Found {
		t.Fatal("A→D 应当可达")
	}
	if res.TotalCost != 3 {
		t.Errorf("期望总代价 3，实际 %v", res.TotalCost)
	}
	if res.Hops != 2 {
		t.Errorf("期望 2 跳，实际 %d", res.Hops)
	}

	wantPath := []NodeID{"A", "B", "D"}
	if got := nodeIDs(res.Nodes); len(got) != len(wantPath) {
		t.Fatalf("期望路径 %v，实际 %v", wantPath, got)
	} else {
		for i := range wantPath {
			if got[i] != wantPath[i] {
				t.Fatalf("期望路径 %v，实际 %v", wantPath, got)
			}
		}
	}

	// Edges[i] 必须真正连接 Nodes[i] 与 Nodes[i+1]，否则前端高亮会错位。
	for i, e := range res.Edges {
		from, to := res.Nodes[i].ID, res.Nodes[i+1].ID
		if !(e.Source == from && e.Target == to) && !(e.Source == to && e.Target == from) {
			t.Errorf("第 %d 条边 %s(%s→%s) 与节点序列 %s→%s 不匹配", i, e.ID, e.Source, e.Target, from, to)
		}
	}
}

// TestShortestPathPrefersLowWeight 权重必须真正影响选路，
// 否则 Dijkstra 就退化成了 BFS。
func TestShortestPathPrefersLowWeight(t *testing.T) {
	g := mustGraph(t)

	// app-web→svc-user 有两条路：直连权重 4，经 api-gw 为 1+2=3。
	res, err := g.ShortestPath(t.Context(), PathOptions{From: "app-web", To: "svc-user", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("ShortestPath 失败: %v", err)
	}
	if res.TotalCost != 3 {
		t.Errorf("期望走 2 跳低权路径（代价 3），实际代价 %v，路径 %v", res.TotalCost, nodeIDs(res.Nodes))
	}
	if res.Hops != 2 {
		t.Errorf("期望 2 跳，实际 %d 跳", res.Hops)
	}
}

func TestShortestPathSameNode(t *testing.T) {
	g := mustGraph(t)
	res, err := g.ShortestPath(t.Context(), PathOptions{From: "app-web", To: "app-web"})
	if err != nil {
		t.Fatalf("ShortestPath 失败: %v", err)
	}
	if !res.Found || res.Hops != 0 || res.TotalCost != 0 {
		t.Errorf("起点即终点应返回零跳平凡路径，实际 found=%v hops=%d cost=%v", res.Found, res.Hops, res.TotalCost)
	}
	if len(res.Nodes) != 1 {
		t.Errorf("期望路径仅含 1 个节点，实际 %d", len(res.Nodes))
	}
}

func TestShortestPathUnreachable(t *testing.T) {
	g := mustGraph(t)
	// db-user 无出边，反向不可达 app-web。
	res, err := g.ShortestPath(t.Context(), PathOptions{From: "db-user", To: "app-web", Direction: DirectionOut})
	if err != nil {
		t.Fatalf("不可达不应报错，而应返回 Found=false: %v", err)
	}
	if res.Found {
		t.Error("db-user 沿出边不应到达 app-web")
	}
}

func TestShortestPathNodeNotFound(t *testing.T) {
	g := mustGraph(t)
	for _, tc := range []PathOptions{
		{From: "ghost", To: "app-web"},
		{From: "app-web", To: "ghost"},
	} {
		if _, err := g.ShortestPath(t.Context(), tc); err == nil {
			t.Errorf("端点 %v 不存在时应报错", tc)
		}
	}
}

func TestShortestPathRelationFilter(t *testing.T) {
	g := mustGraph(t)
	// 限定只走 calls 时，reads_from 连接的 db-user 不可达。
	res, err := g.ShortestPath(t.Context(), PathOptions{
		From: "app-web", To: "db-user", Direction: DirectionOut,
		Relations: []RelationType{RelCalls},
	})
	if err != nil {
		t.Fatalf("ShortestPath 失败: %v", err)
	}
	if res.Found {
		t.Error("仅允许 calls 关系时不应到达 db-user")
	}
}

func TestShortestPathBidirectional(t *testing.T) {
	g := mustGraph(t)
	// 双向模式下无视边方向，db-order 与 srv-01 之间应存在通路。
	res, err := g.ShortestPath(t.Context(), PathOptions{From: "db-order", To: "srv-01", Direction: DirectionBoth})
	if err != nil {
		t.Fatalf("ShortestPath 失败: %v", err)
	}
	if !res.Found {
		t.Fatal("双向遍历下 db-order 与 srv-01 应连通")
	}
	// db-order→svc-order→api-gw→svc-user→srv-01 = 1+3+2+1 = 7
	if res.TotalCost != 7 {
		t.Errorf("期望代价 7，实际 %v，路径 %v", res.TotalCost, nodeIDs(res.Nodes))
	}
}

func TestShortestPathContextCancel(t *testing.T) {
	g := mustGraph(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.ShortestPath(ctx, PathOptions{From: "app-web", To: "db-order"}); err == nil {
		t.Fatal("已取消的 context 应中止查询")
	}
}

// TestMinHeapOrdering 堆必须严格按距离升序弹出。
func TestMinHeapOrdering(t *testing.T) {
	h := newMinHeap(8)
	input := []struct {
		id string
		d  float64
	}{
		{"e", 5}, {"a", 1}, {"c", 3}, {"b", 2}, {"d", 4},
	}
	for _, it := range input {
		h.Push(it.id, it.d)
	}
	if h.Len() != 5 {
		t.Fatalf("期望堆内 5 个元素，实际 %d", h.Len())
	}

	var prev float64 = -1
	for h.Len() > 0 {
		_, d, ok := h.Pop()
		if !ok {
			t.Fatal("Pop 意外失败")
		}
		if d < prev {
			t.Fatalf("弹出顺序违反最小堆性质: %v 出现在 %v 之后", d, prev)
		}
		prev = d
	}
	if _, _, ok := h.Pop(); ok {
		t.Fatal("空堆的 Pop 应返回 false")
	}
}

// TestMinHeapDecreaseKey 重复 Push 同一节点应触发 decrease-key 而非重复入堆。
// 若退化为重复入堆，堆规模会从 O(V) 膨胀到 O(E)。
func TestMinHeapDecreaseKey(t *testing.T) {
	h := newMinHeap(4)
	h.Push("x", 10)
	h.Push("y", 5)
	h.Push("x", 1) // 下调 x 的距离

	if h.Len() != 2 {
		t.Fatalf("decrease-key 不应增加堆规模，期望 2，实际 %d", h.Len())
	}
	id, d, _ := h.Pop()
	if id != "x" || d != 1 {
		t.Fatalf("期望先弹出 x(1)，实际 %s(%v)", id, d)
	}

	// 上调距离应被忽略，Dijkstra 中已确定的更短距离不能被覆盖。
	h.Push("y", 99)
	id, d, _ = h.Pop()
	if id != "y" || d != 5 {
		t.Fatalf("上调距离应被忽略，期望 y(5)，实际 %s(%v)", id, d)
	}
}

func TestMinHeapContains(t *testing.T) {
	h := newMinHeap(2)
	h.Push("a", 1)
	if !h.Contains("a") {
		t.Error("Contains(a) 应为 true")
	}
	h.Pop()
	if h.Contains("a") {
		t.Error("弹出后 Contains(a) 应为 false")
	}
}

// TestMinHeapStress 随机压力测试，验证堆在大量乱序插入下仍保持有序弹出。
func TestMinHeapStress(t *testing.T) {
	const n = 5000
	rng := rand.New(rand.NewSource(42))
	h := newMinHeap(n)

	for i := 0; i < n; i++ {
		h.Push(NodeID(rune('a'+i%26))+NodeID(string(rune(i))), rng.Float64()*1000)
	}

	var prev float64 = -1
	count := 0
	for h.Len() > 0 {
		_, d, _ := h.Pop()
		if d < prev {
			t.Fatalf("第 %d 次弹出违反堆序: %v < %v", count, d, prev)
		}
		prev = d
		count++
	}
	if count == 0 {
		t.Fatal("未弹出任何元素")
	}
}

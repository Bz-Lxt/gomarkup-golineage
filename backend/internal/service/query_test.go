package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
)

// TestWithQueryTimeoutPropagatesCancellation 验证客户端取消请求时，
// 图计算会被及时中止，而非继续运行到自身超时。
//
// 旧实现用 context.WithoutCancel 切断了请求上下文的取消传播链，
// 导致前端连续调整最短路径条件、取消上一条请求后，服务端的旧查询
// 仍在稠密图上跑满 CPU 直到自身超时。这里用一个已取消的 context
// 模拟「请求已被客户端取消」的场景。
func TestWithQueryTimeoutPropagatesCancellation(t *testing.T) {
	cfg := &config.Config{
		GraphMaxDepth:    10,
		GraphMaxPaths:    1000,
		GraphMaxNodes:    50000,
		GraphQueryTimout: 30 * time.Second,
	}
	g := graph.New(graph.DefaultLimits())
	src, sink := buildDenseGraph(t, g)

	svc := New(repository.NewMemoryGraphAdapter(g), eventstore.NewMemoryStore(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	// 模拟前端取消上一条请求：发起后立即取消。
	cancel()

	_, err := svc.ShortestPath(ctx, PathQuery{
		From: string(src), To: string(sink), Direction: "out",
	})
	if err == nil {
		t.Fatal("请求已取消，ShortestPath 应立即返回错误而非继续计算")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 context.Canceled，实际 %v", err)
	}
}

// TestWithQueryTimeoutStillEnforcesTimeout 验证修复不会破坏已有的超时兜底机制：
// 即便客户端不取消，查询超时后也应中止计算。
func TestWithQueryTimeoutStillEnforcesTimeout(t *testing.T) {
	cfg := &config.Config{
		GraphMaxDepth:    10,
		GraphMaxPaths:    1000,
		GraphMaxNodes:    50000,
		// 1ms 足以让 200 节点稠密图的 Dijkstra（约 17ms）在首轮 ctx.Err() 检查处中止。
		GraphQueryTimout: time.Millisecond,
	}
	g := graph.New(graph.DefaultLimits())
	src, sink := buildDenseGraph(t, g)

	svc := New(repository.NewMemoryGraphAdapter(g), eventstore.NewMemoryStore(), cfg)

	_, err := svc.ShortestPath(context.Background(), PathQuery{
		From: string(src), To: string(sink), Direction: "out",
	})
	if err == nil {
		t.Fatal("查询超时后应返回错误")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("期望 context.DeadlineExceeded，实际 %v", err)
	}
}

// buildDenseGraph 构造一张稠密图，确保 Dijkstra 需要松弛大量节点才能命中终点，
// 从而让取消/超时有足够的循环迭代来触发。
// 返回起点与终点的 ID（二者在图中真实存在，且无直连边以保证需要遍历）。
func buildDenseGraph(t *testing.T, g *graph.Graph) (src, sink graph.NodeID) {
	t.Helper()
	const n = 200
	for i := 0; i < n; i++ {
		id := graph.NodeID(strconv.Itoa(i))
		if err := g.AddNode(&graph.Node{ID: id, Name: "节点" + strconv.Itoa(i), Type: graph.NodeTypeApplication}); err != nil {
			t.Fatalf("AddNode(%s) 失败: %v", id, err)
		}
	}
	// 全连接：每对节点之间都有一条有向边，构成稠密图。
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			e := &graph.Edge{
				ID:       graph.EdgeID("e" + strconv.Itoa(i) + "-" + strconv.Itoa(j)),
				Source:   graph.NodeID(strconv.Itoa(i)),
				Target:   graph.NodeID(strconv.Itoa(j)),
				Relation: graph.RelCalls,
				Weight:   1,
				Directed: true,
			}
			if err := g.AddEdge(e); err != nil {
				t.Fatalf("AddEdge(%s) 失败: %v", e.ID, err)
			}
		}
	}
	return graph.NodeID("0"), graph.NodeID("199")
}

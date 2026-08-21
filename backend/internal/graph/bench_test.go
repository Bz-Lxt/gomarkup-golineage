package graph

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
)

// buildLargeGraph 构造规模化的合成资产图，用于性能基线测量。
//
// 拓扑刻意模拟真实企业资产：少量高扇出的枢纽（网关、共享数据库）
// 加上大量低度数的叶子节点，而不是均匀随机图 —— 均匀图的最短路径
// 长度分布与真实血缘图差异很大，测出的数字没有参考价值。
func buildLargeGraph(tb testing.TB, nodeCount, edgeCount int) *Graph {
	tb.Helper()

	g := New(Limits{MaxDepth: 10, MaxPaths: 1000, MaxNodes: 200000})
	types := AllNodeTypes()

	for i := 0; i < nodeCount; i++ {
		id := fmt.Sprintf("n%07d", i)
		n := &Node{
			ID:   id,
			Name: fmt.Sprintf("资产-%d", i),
			Type: types[i%len(types)],
			Properties: Properties{
				"ip":    fmt.Sprintf("10.%d.%d.%d", i/65536%256, i/256%256, i%256),
				"owner": fmt.Sprintf("负责人%d", i%50),
			},
		}
		if err := g.AddNode(n); err != nil {
			tb.Fatalf("构建大图失败: %v", err)
		}
	}

	rng := rand.New(rand.NewSource(20260821))
	hubCount := nodeCount / 100
	if hubCount < 1 {
		hubCount = 1
	}
	rels := AllRelationTypes()

	added := 0
	for i := 0; added < edgeCount && i < edgeCount*4; i++ {
		var src, dst int
		// 三成的边挂到枢纽上，形成长尾度数分布。
		if rng.Intn(10) < 3 {
			src = rng.Intn(hubCount)
			dst = rng.Intn(nodeCount)
		} else {
			src = rng.Intn(nodeCount)
			dst = rng.Intn(nodeCount)
		}
		if src == dst {
			continue
		}
		e := &Edge{
			ID:       fmt.Sprintf("e%08d", i),
			Source:   fmt.Sprintf("n%07d", src),
			Target:   fmt.Sprintf("n%07d", dst),
			Relation: rels[i%len(rels)],
			Weight:   1 + rng.Float64()*9,
			Directed: true,
		}
		if err := g.AddEdge(e); err == nil {
			added++
		}
	}
	return g
}

const (
	benchNodes = 100000
	benchEdges = 500000
)

func BenchmarkBuildGraph(b *testing.B) {
	for i := 0; i < b.N; i++ {
		buildLargeGraph(b, 10000, 50000)
	}
}

func BenchmarkNeighbors1Hop(b *testing.B) {
	g := buildLargeGraph(b, benchNodes, benchEdges)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("n%07d", i%benchNodes)
		if _, err := g.NeighborSubgraph(ctx, id, 1, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBFS3Hop(b *testing.B) {
	g := buildLargeGraph(b, benchNodes, benchEdges)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := fmt.Sprintf("n%07d", i%benchNodes)
		if _, err := g.BFS(ctx, TraverseOptions{
			Start: id, MaxDepth: 3, Direction: DirectionOut, MaxNodes: 5000,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDijkstra(b *testing.B) {
	g := buildLargeGraph(b, benchNodes, benchEdges)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(7))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		from := fmt.Sprintf("n%07d", rng.Intn(benchNodes))
		to := fmt.Sprintf("n%07d", rng.Intn(benchNodes))
		if _, err := g.ShortestPath(ctx, PathOptions{From: from, To: to, Direction: DirectionOut}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAddNode(b *testing.B) {
	g := New(DefaultLimits())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.AddNode(&Node{
			ID:   fmt.Sprintf("bench%09d", i),
			Name: "压测节点",
			Type: NodeTypeServer,
		})
	}
}

func BenchmarkSearchByType(b *testing.B) {
	g := buildLargeGraph(b, benchNodes, benchEdges)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Search(SearchOptions{Types: []NodeType{NodeTypeDatabase}, Limit: 100})
	}
}

func BenchmarkSnapshot(b *testing.B) {
	g := buildLargeGraph(b, 10000, 50000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.Snapshot()
	}
}

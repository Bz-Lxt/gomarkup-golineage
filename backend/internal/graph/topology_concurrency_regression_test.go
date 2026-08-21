package graph_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestTopologyRemainsSafeDuringConcurrentUpdates(t *testing.T) {
	const nodeCount = 800

	g := graph.New(graph.Limits{MaxDepth: 10, MaxPaths: 100, MaxNodes: nodeCount})
	for i := 0; i < nodeCount; i++ {
		n := &graph.Node{
			ID:   fmt.Sprintf("asset-%04d", i),
			Name: fmt.Sprintf("资产 %04d", i),
			Type: graph.NodeTypeApplication,
		}
		if err := g.AddNode(n); err != nil {
			t.Fatalf("初始化资产失败: %v", err)
		}
	}

	start := make(chan struct{})
	errCh := make(chan error, 6)
	var wg sync.WaitGroup

	wg.Add(2)
	for writer := 0; writer < 2; writer++ {
		go func(writer int) {
			defer wg.Done()
			<-start
			for i := 0; i < 600; i++ {
				id := (i*17 + writer) % nodeCount
				n := &graph.Node{
					ID:   fmt.Sprintf("asset-%04d", id),
					Name: fmt.Sprintf("资产 %04d 第 %d 轮同步", id, i),
					Type: graph.NodeTypeApplication,
				}
				if err := g.PutNode(n); err != nil {
					errCh <- fmt.Errorf("更新资产失败: %w", err)
					return
				}
			}
		}(writer)
	}

	wg.Add(4)
	for reader := 0; reader < 4; reader++ {
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 80; i++ {
				topology := g.Topology(graph.TopologyOptions{Limit: nodeCount})
				if topology.TotalNodes != nodeCount || len(topology.Nodes) != nodeCount {
					errCh <- fmt.Errorf("拓扑资产数不完整: total=%d returned=%d", topology.TotalNodes, len(topology.Nodes))
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

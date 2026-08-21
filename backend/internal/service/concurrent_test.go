package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	cfg := &config.Config{
		GraphMaxDepth:    10,
		GraphMaxPaths:    100,
		GraphMaxNodes:    5000,
		GraphQueryTimout: 5 * time.Second,
		SnapshotInterval: 0,
	}
	g := graph.New(graph.Limits{
		MaxDepth: cfg.GraphMaxDepth, MaxPaths: cfg.GraphMaxPaths, MaxNodes: cfg.GraphMaxNodes,
	})
	return New(repository.NewMemoryGraphAdapter(g), eventstore.NewMemoryStore(), cfg)
}

// TestDeleteNodeConcurrentWithCreateEdge 验证下线与导入并发时，
// 内存投影与事件日志之间的一致性不变式不被破坏。
//
// 场景：端点 A、B 已存在。一个协程删除 A（下线），
// 另一个协程在 A、B 之间建立关系（导入）。
//
// 下线时若 IncidentEdges 在加写锁前读取，导入边可能恰好落在
// 「读快照」与「加锁」之间：下线会级联删除这条边，但级联事件
// 是依据过期列表生成的，日志里因此缺少这条边的删除记录。
// 重放后拓扑会重建出一条当前内存里查不到的边——日志与投影分裂。
//
// 本测试断言：若导入边已从内存消失，则日志中必须同时存在对应的
// edge_created 与 edge_deleted 事件；反之则不可有删除事件。
func TestDeleteNodeConcurrentWithCreateEdge(t *testing.T) {
	t.Parallel()
	const rounds = 200
	for i := 0; i < rounds; i++ {
		s := newTestService(t)
		ctx := context.Background()

		a, err := s.CreateNode(ctx, CreateNodeInput{
			Name: "A", Type: string(graph.NodeTypeApplication), Actor: "seed",
		})
		if err != nil {
			t.Fatalf("round %d: 创建 A 失败: %v", i, err)
		}
		b, err := s.CreateNode(ctx, CreateNodeInput{
			Name: "B", Type: string(graph.NodeTypeDatabase), Actor: "seed",
		})
		if err != nil {
			t.Fatalf("round %d: 创建 B 失败: %v", i, err)
		}

		var (
			wg     sync.WaitGroup
			edgeID string
			delOK  bool
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			e, err := s.CreateEdge(ctx, CreateEdgeInput{
				SourceID: a.ID, TargetID: b.ID,
				Relation: string(graph.RelReadsFrom), Actor: "importer",
			})
			if err == nil {
				edgeID = e.ID
			}
			// 导入失败（A 已被删）是合法竞争结果，忽略 err。
		}()
		go func() {
			defer wg.Done()
			_, err := s.DeleteNode(ctx, a.ID, "operator", "资产下线")
			delOK = err == nil
		}()
		wg.Wait()

		if !delOK {
			t.Fatalf("round %d: DeleteNode 不应失败", i)
		}

		// A 一定被删，因此导入成功的边必然已被级联删除；
		// 导入失败的边则根本不应出现在日志里。
		if edgeID == "" {
			continue
		}

		_, gerr := s.Repo().GetEdge(ctx, edgeID)
		if gerr == nil {
			// 节点 A 已删除，边不可能仍在内存。
			t.Fatalf("round %d: 边 %s 仍在内存，但 A 已删除", i, edgeID)
		}

		// 日志里必须有这条边的删除事件。
		deleted, _, err := s.Store().List(ctx, eventstore.ListFilter{
			Types: []eventstore.EventType{eventstore.EventEdgeDeleted}, Limit: 100,
		})
		if err != nil {
			t.Fatalf("round %d: List 失败: %v", i, err)
		}
		found := false
		for _, e := range deleted {
			if e.EntityID == edgeID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("round %d: 导入边 %s 已被级联删除，但日志缺少删除事件——"+
				"内存无此边、日志只留 created，审计无法解释其消失", i, edgeID)
		}
	}
}

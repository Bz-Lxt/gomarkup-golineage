package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
	"github.com/alkaid/golineage/internal/service"
)

type incidentGateRepository struct {
	repository.GraphRepository
	target  string
	reached chan struct{}
	release chan struct{}
}

func (r *incidentGateRepository) IncidentEdges(ctx context.Context, id string) ([]*graph.Edge, error) {
	edges, err := r.GraphRepository.IncidentEdges(ctx, id)
	if id == r.target {
		select {
		case r.reached <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return edges, err
}

func TestDeleteNodeConcurrentEdgeCreateRecordsCascade(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	g := graph.New(graph.DefaultLimits())
	baseRepo := repository.NewMemoryGraphAdapter(g)
	repo := &incidentGateRepository{
		GraphRepository: baseRepo,
		reached:         make(chan struct{}),
		release:         make(chan struct{}),
	}
	store := eventstore.NewMemoryStore()
	svc := service.New(repo, store, &config.Config{SnapshotInterval: 0})

	victim, err := svc.CreateNode(ctx, service.CreateNodeInput{Name: "待下线数据库", Type: string(graph.NodeTypeDatabase)})
	if err != nil {
		t.Fatalf("创建待删除资产失败: %v", err)
	}
	caller, err := svc.CreateNode(ctx, service.CreateNodeInput{Name: "导入中的应用", Type: string(graph.NodeTypeApplication)})
	if err != nil {
		t.Fatalf("创建调用方资产失败: %v", err)
	}
	repo.target = victim.ID

	deleteDone := make(chan error, 1)
	go func() {
		_, err := svc.DeleteNode(ctx, victim.ID, "cleanup", "下线资产")
		deleteDone <- err
	}()

	select {
	case <-repo.reached:
	case <-ctx.Done():
		t.Fatal("删除操作未进入级联关系读取阶段")
	}

	type createResult struct {
		edge *graph.Edge
		err  error
	}
	createDone := make(chan createResult, 1)
	go func() {
		edge, err := svc.CreateEdge(ctx, service.CreateEdgeInput{
			SourceID: caller.ID,
			TargetID: victim.ID,
			Relation: string(graph.RelCalls),
			Actor:    "importer",
		})
		createDone <- createResult{edge: edge, err: err}
	}()

	var created createResult
	createFinished := false
	select {
	case created = <-createDone:
		createFinished = true
		if created.err != nil {
			t.Fatalf("删除尚未提交时并发创建关系失败: %v", created.err)
		}
	case <-time.After(50 * time.Millisecond):
	}
	close(repo.release)

	if err := <-deleteDone; err != nil {
		t.Fatalf("删除资产失败: %v", err)
	}
	if !createFinished {
		created = <-createDone
	}

	if created.err == nil {
		events, _, err := store.List(ctx, eventstore.ListFilter{EntityID: created.edge.ID, Limit: 10})
		if err != nil {
			t.Fatalf("查询关系流水失败: %v", err)
		}
		var deleted bool
		for _, event := range events {
			if event.Type == eventstore.EventEdgeDeleted {
				deleted = true
			}
		}
		if !deleted {
			t.Fatalf("并发创建成功的关系被资产删除级联清理后，流水中缺少关系删除记录: edge_id=%s", created.edge.ID)
		}
	}

	replayed := graph.New(graph.DefaultLimits())
	if _, err := eventstore.ReplayLive(ctx, store, replayed); err != nil {
		t.Fatalf("并发写入后的流水应能在重启时完整重放: %v", err)
	}
	if replayed.NodeCount() != g.NodeCount() || replayed.EdgeCount() != g.EdgeCount() {
		t.Fatalf("重放拓扑与在线拓扑不一致: 在线 nodes=%d edges=%d, 重放 nodes=%d edges=%d",
			g.NodeCount(), g.EdgeCount(), replayed.NodeCount(), replayed.EdgeCount())
	}
}

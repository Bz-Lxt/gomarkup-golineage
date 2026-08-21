package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
	"github.com/alkaid/golineage/internal/service"
)

type failSecondRangeStore struct {
	eventstore.Store
	rangeCalls int
}

func (s *failSecondRangeStore) Range(ctx context.Context, fromSeq int64, until *time.Time) ([]*eventstore.Event, error) {
	s.rangeCalls++
	if s.rangeCalls == 2 {
		return nil, errors.New("event store read interrupted")
	}
	return s.Store.Range(ctx, fromSeq, until)
}

func TestDiffTopologyPropagatesEndSnapshotReadError(t *testing.T) {
	memoryStore := eventstore.NewMemoryStore()
	store := &failSecondRangeStore{Store: memoryStore}
	limits := graph.Limits{MaxDepth: 10, MaxPaths: 100, MaxNodes: 100}
	svc := service.New(
		repository.NewMemoryGraphAdapter(graph.New(limits)),
		store,
		&config.Config{GraphQueryTimout: time.Second},
	)

	from := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	node := &graph.Node{
		ID: "asset-1", Name: "order-service", Type: graph.NodeTypeApplication,
		Properties: graph.Properties{}, CreatedAt: from.Add(30 * time.Minute), UpdatedAt: from.Add(30 * time.Minute),
	}
	event, err := eventstore.NewNodeEvent(eventstore.EventNodeCreated, node, nil, "tester", "import", node.CreatedAt)
	if err != nil {
		t.Fatalf("构造新增事件失败: %v", err)
	}
	if err := memoryStore.Append(t.Context(), []*eventstore.Event{event}); err != nil {
		t.Fatalf("写入新增事件失败: %v", err)
	}

	diff, err := svc.DiffTopology(t.Context(), from.Format(time.RFC3339), to.Format(time.RFC3339))
	if err == nil {
		t.Fatalf("结束时刻的事件读取失败应向调用方返回错误，实际成功返回差异: %+v", diff)
	}
}

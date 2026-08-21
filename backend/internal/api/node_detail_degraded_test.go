package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
	"github.com/alkaid/golineage/internal/service"
)

type listFailingStore struct {
	*eventstore.MemoryStore
}

func (s *listFailingStore) List(context.Context, eventstore.ListFilter) ([]*eventstore.Event, int64, error) {
	return nil, 0, errors.New("event store query unavailable")
}

func TestNodeDetailSurvivesEventListFailure(t *testing.T) {
	cfg := &config.Config{
		GraphMaxDepth:    10,
		GraphMaxPaths:    100,
		GraphMaxNodes:    5000,
		GraphQueryTimout: time.Second,
	}
	g := graph.New(graph.Limits{MaxDepth: 10, MaxPaths: 100, MaxNodes: 5000})
	node := &graph.Node{ID: "asset-1", Name: "订单服务", Type: graph.NodeTypeApplication}
	if err := g.AddNode(node); err != nil {
		t.Fatalf("prepare node: %v", err)
	}
	store := &listFailingStore{MemoryStore: eventstore.NewMemoryStore()}
	svc := service.New(repository.NewMemoryGraphAdapter(g), store, cfg)
	h := NewHandler(svc).Router(nil, 2*time.Second)

	status, resp := call(t, h, http.MethodGet, "/api/v1/nodes/asset-1", nil)
	if status != http.StatusOK {
		t.Fatalf("event count failure should not hide graph-backed node detail: status=%d resp=%+v", status, resp)
	}
	var detail service.NodeDetail
	dataAs(t, resp, &detail)
	if detail.Node == nil || detail.Node.ID != "asset-1" {
		t.Fatalf("expected requested node detail, got %+v", detail.Node)
	}
	if detail.EventCount != 0 {
		t.Fatalf("unavailable event count should degrade to zero, got %d", detail.EventCount)
	}
}

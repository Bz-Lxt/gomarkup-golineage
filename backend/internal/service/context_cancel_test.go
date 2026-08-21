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

func TestShortestPathStopsWhenRequestCanceled(t *testing.T) {
	g := graph.New(graph.Limits{MaxDepth: 10, MaxPaths: 100, MaxNodes: 100})
	svc := service.New(repository.NewMemoryGraphAdapter(g), eventstore.NewMemoryStore(), &config.Config{
		GraphQueryTimout: 5 * time.Second,
	})

	from, err := svc.CreateNode(context.Background(), service.CreateNodeInput{Name: "cancel-source", Type: "application"})
	if err != nil {
		t.Fatal(err)
	}
	to, err := svc.CreateNode(context.Background(), service.CreateNodeInput{Name: "cancel-target", Type: "database"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateEdge(context.Background(), service.CreateEdgeInput{
		SourceID: from.ID, TargetID: to.ID, Relation: "reads_from",
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = svc.ShortestPath(ctx, service.PathQuery{From: from.ID, To: to.ID, Direction: "out"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消的查询应返回 context.Canceled，实际 err=%v", err)
	}
}

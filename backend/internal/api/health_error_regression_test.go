package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
	"github.com/alkaid/golineage/internal/service"
)

type pingFailingStore struct {
	*eventstore.MemoryStore
}

func (s *pingFailingStore) Ping(context.Context) error {
	return errors.New("connection refused")
}

func TestHealthzReportsPingFailureWhenCountStillSucceeds(t *testing.T) {
	cfg := &config.Config{SnapshotInterval: 0}
	g := graph.New(graph.Limits{MaxDepth: 10, MaxPaths: 100, MaxNodes: 100})
	store := &pingFailingStore{MemoryStore: eventstore.NewMemoryStore()}
	svc := service.New(repository.NewMemoryGraphAdapter(g), store, cfg)
	h := NewHandler(svc).Router(nil, time.Second)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("event store ping failed but health endpoint returned status %d: %s", rec.Code, rec.Body.String())
	}
}

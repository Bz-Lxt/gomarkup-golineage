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

type rangeErrorStore struct {
	eventstore.Store
	err error
}

func (s *rangeErrorStore) Range(context.Context, int64, *time.Time) ([]*eventstore.Event, error) {
	return nil, s.err
}

func TestReplayFromLogReportsEventReadFailure(t *testing.T) {
	readErr := errors.New("event stream temporarily unavailable")
	store := &rangeErrorStore{Store: eventstore.NewMemoryStore(), err: readErr}
	g := graph.New(graph.Limits{MaxDepth: 10, MaxPaths: 100, MaxNodes: 1000})
	svc := service.New(repository.NewMemoryGraphAdapter(g), store, &config.Config{})

	stats, err := svc.ReplayFromLog(t.Context())
	if !errors.Is(err, readErr) {
		t.Fatalf("事件流读取失败应向启动调用方返回原始错误，stats=%+v err=%v", stats, err)
	}
}

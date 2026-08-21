// Command server 是 GoLineage 后端服务的入口。
//
// 启动顺序刻意设计为：连接事件存储 → 按需写入种子 → 从事件日志重放重建内存图 → 对外提供服务。
// 内存图只有这一条构建路径，不存在任何旁路恢复，因此重启前后的拓扑必然一致。
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alkaid/golineage/internal/api"
	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
	"github.com/alkaid/golineage/internal/seed"
	"github.com/alkaid/golineage/internal/service"
	"github.com/alkaid/golineage/internal/timeutil"
	"github.com/alkaid/golineage/pkg/logger"
)

// shutdownTimeout 优雅关闭的最长等待时间。
const shutdownTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		logger.Error("服务启动失败", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger.Init(logger.ParseLevel(cfg.LogLevel), os.Stdout)
	logger.Info("GoLineage 服务启动中",
		"addr", cfg.HTTPAddr, "log_level", cfg.LogLevel,
		"server_time", timeutil.Format(timeutil.Now()))

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancelStart := context.WithTimeout(rootCtx, cfg.DBConnectTimeout+30*time.Second)
	defer cancelStart()

	store, err := eventstore.NewPostgresStore(startCtx, cfg.DBDSN, cfg.DBMaxConns, cfg.DBConnectTimeout)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := maybeSeed(startCtx, store, cfg); err != nil {
		return err
	}

	g := graph.New(graph.Limits{
		MaxDepth: cfg.GraphMaxDepth,
		MaxPaths: cfg.GraphMaxPaths,
		MaxNodes: cfg.GraphMaxNodes,
	})
	repo := repository.NewMemoryGraphAdapter(g)
	svc := service.New(repo, store, cfg)

	stats, err := svc.ReplayFromLog(startCtx)
	if err != nil {
		return err
	}
	logger.Info("内存图重建完成",
		"events_applied", stats.EventsApplied, "used_checkpoint", stats.UsedCheckpoint,
		"nodes", stats.NodeCount, "edges", stats.EdgeCount, "cost_ms", stats.DurationMS)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.NewHandler(svc).Router(cfg.CORSAllowOrigin, cfg.GraphQueryTimout+5*time.Second),
		// ReadHeaderTimeout 防止 slowloris 类型的慢速请求头攻击。
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      cfg.GraphQueryTimout + 20*time.Second,
		IdleTimeout:       90 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("HTTP 服务已就绪", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-rootCtx.Done():
		logger.Info("收到退出信号，开始优雅关闭")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("优雅关闭超时，将强制退出", "err", err)
	}
	logger.Info("服务已停止")
	return nil
}

// maybeSeed 仅在事件日志为空时写入演示数据。
//
// 判空而非清库重建：一旦有真实变更流水，任何自动写入都可能污染历史，
// 而事件日志是不可变的权威数据源，污染无法回退。
func maybeSeed(ctx context.Context, store eventstore.Store, cfg *config.Config) error {
	if !cfg.SeedOnEmpty || cfg.SeedScenario == seed.ScenarioNone {
		logger.Info("跳过种子数据写入", "seed_on_empty", cfg.SeedOnEmpty, "scenario", cfg.SeedScenario)
		return nil
	}

	count, err := store.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		logger.Info("事件日志非空，跳过种子数据写入", "existing_events", count)
		return nil
	}

	res, err := seed.Apply(ctx, store, cfg.SeedScenario)
	if err != nil {
		return err
	}
	logger.Info("种子数据初始化完成", "scenario", res.Scenario, "events", res.EventCount)
	return nil
}

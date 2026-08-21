// Package service 编排图引擎与事件溯源，是业务规则的唯一落点。
//
// 核心不变式（写路径）：
//  1. 先在内存投影上做预检查，把可预见的业务错误挡在写日志之前；
//  2. 事件成功落盘之后，才应用到内存图；
//  3. 全部写操作串行执行，保证「日志顺序 == 应用顺序」。
//
// 第 3 条尤为关键：若并发写入，两个请求可能都通过预检查、都写入日志，
// 而第二个在应用时失败，内存投影就与权威日志产生了分歧。
// 图谱类系统读远多于写，用一把互斥锁换取这个不变式非常划算。
package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/alkaid/golineage/internal/config"
	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/repository"
	"github.com/alkaid/golineage/internal/timeutil"
	"github.com/alkaid/golineage/pkg/logger"
)

// Service 业务编排服务。
type Service struct {
	repo  repository.GraphRepository
	store eventstore.Store
	cfg   *config.Config

	// writeMu 串行化写路径，维护「日志顺序 == 内存应用顺序」不变式。
	writeMu sync.Mutex
}

// New 构造服务实例。
func New(repo repository.GraphRepository, store eventstore.Store, cfg *config.Config) *Service {
	return &Service{repo: repo, store: store, cfg: cfg}
}

// Repo 返回底层图仓储，供只读查询与健康检查使用。
func (s *Service) Repo() repository.GraphRepository { return s.repo }

// Store 返回事件存储。
func (s *Service) Store() eventstore.Store { return s.store }

// Config 返回运行时配置。
func (s *Service) Config() *config.Config { return s.cfg }

// commit 持久化事件并应用到内存投影。调用方必须已持有 writeMu。
//
// apply 在事件落盘之后执行。此处失败意味着内存投影落后于权威日志，
// 属于严重不一致：记录 ERROR 并立即从日志全量重建，宁可短暂卡顿，
// 也不能让后续查询基于一份错误的拓扑给出结论。
func (s *Service) commit(ctx context.Context, events []*eventstore.Event, apply func() error) error {
	if err := s.store.Append(ctx, events); err != nil {
		return fmt.Errorf("写入变更流水失败: %w", err)
	}

	if err := apply(); err != nil {
		logger.ErrorCtx(ctx, "事件已落盘但应用到内存图失败，立即从日志重建投影",
			"err", err, "event_count", len(events))
		if _, rerr := eventstore.ReplayLive(ctx, s.store, s.repo.Underlying()); rerr != nil {
			logger.ErrorCtx(ctx, "内存投影重建失败，服务已进入降级状态", "err", rerr)
		}
		return fmt.Errorf("应用变更到内存图失败: %w", err)
	}

	var lastSeq int64
	for _, e := range events {
		if e.Seq > lastSeq {
			lastSeq = e.Seq
		}
	}
	eventstore.MaybeCheckpoint(ctx, s.store, s.repo.Underlying(), lastSeq, s.cfg.SnapshotInterval)
	return nil
}

// ReplayFromLog 从事件日志重建内存投影，用于启动恢复。
func (s *Service) ReplayFromLog(ctx context.Context) (*eventstore.ReplayStats, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return eventstore.ReplayLive(ctx, s.store, s.repo.Underlying())
}

// newID 生成实体标识。
func newID() string { return uuid.NewString() }

// normalizeActor 归一化操作者，空值记为匿名而非空串，
// 否则流水面板会出现无主的变更记录。
func normalizeActor(a string) string {
	if a = strings.TrimSpace(a); a != "" {
		return a
	}
	return "anonymous"
}

// normalizeReason 裁剪超长的变更原因，避免单条流水撑爆列表渲染。
func normalizeReason(r string) string {
	r = strings.TrimSpace(r)
	if rs := []rune(r); len(rs) > eventstore.MaxReasonLength {
		return string(rs[:eventstore.MaxReasonLength])
	}
	return r
}

// parseNodeTypes 解析节点类型列表，任一非法即整体拒绝。
func parseNodeTypes(raw []string) ([]graph.NodeType, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]graph.NodeType, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		t := graph.NodeType(s)
		if !t.Valid() {
			return nil, fmt.Errorf("%w: 节点类型 %q 非法", graph.ErrValidation, s)
		}
		out = append(out, t)
	}
	return out, nil
}

// parseRelationTypes 解析关系类型列表，任一非法即整体拒绝。
func parseRelationTypes(raw []string) ([]graph.RelationType, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]graph.RelationType, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		r := graph.RelationType(s)
		if !r.Valid() {
			return nil, fmt.Errorf("%w: 关系类型 %q 非法", graph.ErrValidation, s)
		}
		out = append(out, r)
	}
	return out, nil
}

// Metadata 前端初始化所需的枚举与配置元数据。
type Metadata struct {
	NodeTypes     []graph.NodeType       `json:"node_types"`
	RelationTypes []graph.RelationType   `json:"relation_types"`
	EventTypes    []metaEventType        `json:"event_types"`
	PropertyKeys  []string               `json:"property_keys"`
	Limits        graph.Limits           `json:"limits"`
	Stats         graph.Stats            `json:"stats"`
	Adapter       string                 `json:"adapter"`
	ServerTime    string                 `json:"server_time"`
	Extra         map[string]interface{} `json:"extra,omitempty"`
}

type metaEventType struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Metadata 返回前端渲染筛选器与表单所需的全部枚举。
//
// 由后端下发而非前端硬编码：枚举新增时只需改一处，
// 前端不会出现「后端已支持但下拉框里没有」的错位。
func (s *Service) Metadata(ctx context.Context) *Metadata {
	events := eventstore.AllEventTypes()
	metaEvents := make([]metaEventType, len(events))
	for i, e := range events {
		metaEvents[i] = metaEventType{Value: string(e), Label: e.Label()}
	}
	return &Metadata{
		NodeTypes:     graph.AllNodeTypes(),
		RelationTypes: graph.AllRelationTypes(),
		EventTypes:    metaEvents,
		PropertyKeys:  s.repo.PropertyKeys(ctx),
		Limits:        s.repo.Limits(),
		Stats:         s.repo.Stats(ctx),
		Adapter:       s.repo.Name(),
		ServerTime:    timeutil.FormatRFC3339(timeutil.Now()),
	}
}

// HealthStatus 健康检查结果。
type HealthStatus struct {
	Status     string `json:"status"`
	Database   string `json:"database"`
	Adapter    string `json:"adapter"`
	NodeCount  int    `json:"node_count"`
	EdgeCount  int    `json:"edge_count"`
	EventCount int64  `json:"event_count"`
	ServerTime string `json:"server_time"`
}

// Health 汇总服务健康状态。
//
// 数据库探测是真实执行的，而不是固定返回 ok —— 一个永远健康的
// 健康检查毫无价值，编排系统会把已经不可用的容器继续留在流量里。
func (s *Service) Health(ctx context.Context) *HealthStatus {
	h := &HealthStatus{
		Status:     "ok",
		Database:   "ok",
		Adapter:    s.repo.Name(),
		ServerTime: timeutil.FormatRFC3339(timeutil.Now()),
	}
	stats := s.repo.Stats(ctx)
	h.NodeCount = stats.NodeCount
	h.EdgeCount = stats.EdgeCount

	if err := s.store.Ping(ctx); err != nil {
		h.Status = "degraded"
		h.Database = "unreachable: " + err.Error()
		return h
	}
	if n, err := s.store.Count(ctx); err == nil {
		h.EventCount = n
	}
	return h
}

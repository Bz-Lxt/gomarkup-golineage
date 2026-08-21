package service

import (
	"context"
	"fmt"
	"time"

	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/timeutil"
)

// EventQuery 变更流水查询参数。
type EventQuery struct {
	EntityID string
	Types    []string
	From     string
	To       string
	Actor    string
	Limit    int
	Offset   int
	Desc     bool
}

// EventPage 分页的变更流水。
type EventPage struct {
	Items  []*eventstore.Event `json:"items"`
	Total  int64               `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

// ListEvents 查询血缘变更流水。
func (s *Service) ListEvents(ctx context.Context, q EventQuery) (*EventPage, error) {
	f := eventstore.ListFilter{
		EntityID: q.EntityID,
		Actor:    q.Actor,
		Limit:    q.Limit,
		Offset:   q.Offset,
		Desc:     q.Desc,
	}

	for _, t := range q.Types {
		et := eventstore.EventType(t)
		if !et.Valid() {
			return nil, fmt.Errorf("%w: 事件类型 %q 非法", graph.ErrValidation, t)
		}
		f.Types = append(f.Types, et)
	}

	var err error
	if f.From, err = parseOptionalTime(q.From, "from"); err != nil {
		return nil, err
	}
	if f.To, err = parseOptionalTime(q.To, "to"); err != nil {
		return nil, err
	}
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return nil, fmt.Errorf("%w: 起始时间不能晚于结束时间", graph.ErrValidation)
	}

	items, total, err := s.store.List(ctx, f)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*eventstore.Event{}
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	return &EventPage{Items: items, Total: total, Limit: limit, Offset: f.Offset}, nil
}

// listFilterForEntity 构造只统计总数的轻量过滤器。
func listFilterForEntity(id string) eventstore.ListFilter {
	return eventstore.ListFilter{EntityID: id, Limit: 1}
}

// EntityTimeline 单个实体的完整变更历史，用于右侧抽屉内的时间线。
func (s *Service) EntityTimeline(ctx context.Context, entityID string, limit int) (*EventPage, error) {
	if entityID == "" {
		return nil, fmt.Errorf("%w: 实体 ID 不能为空", graph.ErrValidation)
	}
	return s.ListEvents(ctx, EventQuery{EntityID: entityID, Limit: limit, Desc: true})
}

// HistoricalTopology 某一时刻的历史拓扑。
type HistoricalTopology struct {
	At       string          `json:"at"`
	Topology *graph.Topology `json:"topology"`
	Replay   *replayMeta     `json:"replay"`
}

type replayMeta struct {
	EventsApplied int   `json:"events_applied"`
	LastSeq       int64 `json:"last_seq"`
	DurationMS    int64 `json:"duration_ms"`
}

// SnapshotAt 重建并返回指定时刻的历史拓扑。
//
// 在独立的图实例上重放，实时视图完全不受影响 ——
// 一位用户拖动时间轴回溯，不应让其他人的查询看到历史数据。
func (s *Service) SnapshotAt(ctx context.Context, atRaw string, limit int) (*HistoricalTopology, error) {
	at, err := parseRequiredTime(atRaw, "at")
	if err != nil {
		return nil, err
	}

	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()

	g, stats, err := eventstore.SnapshotAt(ctx, s.store, *at, s.repo.Limits())
	if err != nil {
		return nil, err
	}
	return &HistoricalTopology{
		At:       timeutil.FormatRFC3339(*at),
		Topology: g.Topology(graph.TopologyOptions{Limit: limit}),
		Replay: &replayMeta{
			EventsApplied: stats.EventsApplied,
			LastSeq:       stats.LastSeq,
			DurationMS:    stats.DurationMS,
		},
	}, nil
}

// DiffTopology 比较两个时刻之间的拓扑差异。
func (s *Service) DiffTopology(ctx context.Context, fromRaw, toRaw string) (*eventstore.TopologyDiff, error) {
	from, err := parseRequiredTime(fromRaw, "from")
	if err != nil {
		return nil, err
	}
	to, err := parseRequiredTime(toRaw, "to")
	if err != nil {
		return nil, err
	}
	if from.After(*to) {
		return nil, fmt.Errorf("%w: 起始时间不能晚于结束时间", graph.ErrValidation)
	}

	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()

	limits := s.repo.Limits()
	before, _, err := eventstore.SnapshotAt(ctx, s.store, *from, limits)
	if err != nil {
		return nil, fmt.Errorf("重建起始时刻拓扑失败: %w", err)
	}
	after, _, err := eventstore.SnapshotAt(ctx, s.store, *to, limits)
	if err != nil {
		after = before
	}

	return eventstore.Diff(before.Snapshot(), after.Snapshot(), *from, *to), nil
}

// TimelineBounds 时间轴的可用范围，供前端初始化滑块。
type TimelineBounds struct {
	Earliest   string `json:"earliest"`
	Latest     string `json:"latest"`
	EventCount int64  `json:"event_count"`
	Available  bool   `json:"available"`
}

// TimelineBounds 返回变更流水的时间跨度。
func (s *Service) TimelineBounds(ctx context.Context) (*TimelineBounds, error) {
	total, err := s.store.Count(ctx)
	if err != nil {
		return nil, err
	}
	b := &TimelineBounds{EventCount: total}
	if total == 0 {
		return b, nil
	}

	first, _, err := s.store.List(ctx, eventstore.ListFilter{Limit: 1})
	if err != nil {
		return nil, err
	}
	last, _, err := s.store.List(ctx, eventstore.ListFilter{Limit: 1, Desc: true})
	if err != nil {
		return nil, err
	}
	if len(first) > 0 && len(last) > 0 {
		b.Earliest = timeutil.FormatRFC3339(first[0].OccurredAt)
		b.Latest = timeutil.FormatRFC3339(last[0].OccurredAt)
		b.Available = true
	}
	return b, nil
}

func parseOptionalTime(raw, field string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := timeutil.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: 参数 %s 的时间格式无法解析（期望 RFC3339 或 2006-01-02 15:04:05）", graph.ErrValidation, field)
	}
	return &t, nil
}

func parseRequiredTime(raw, field string) (*time.Time, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: 参数 %s 不能为空", graph.ErrValidation, field)
	}
	return parseOptionalTime(raw, field)
}

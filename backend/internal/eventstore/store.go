package eventstore

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/alkaid/golineage/internal/graph"
)

// ListFilter 事件流水查询条件。
type ListFilter struct {
	// EntityID 只看某个实体的变更历史（用于属性抽屉内的时间线）。
	EntityID string
	// Types 事件类型过滤，空表示不限。
	Types []EventType
	// From / To 发生时间范围，闭区间，nil 表示不限。
	From *time.Time
	To   *time.Time
	// Actor 操作者过滤。
	Actor string
	// Limit / Offset 分页参数。
	Limit  int
	Offset int
	// Desc 为 true 时按序列号倒序（流水面板默认最新在前）。
	Desc bool
}

func (f ListFilter) normalize() ListFilter {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return f
}

// Store 事件日志与检查点的持久化抽象。
//
// 抽象出接口的意义在于单元测试可以用内存实现离线运行；
// 若直接依赖 pgx，图重放逻辑就只能在有数据库的环境里验证。
type Store interface {
	// Append 原子追加一批事件，并回填各事件的 Seq。
	//
	// 必须是原子的：删除一个节点会同时产生「节点删除」与 N 条「关系删除」事件，
	// 若部分写入成功，重放出的拓扑会残留悬空边。
	Append(ctx context.Context, events []*Event) error

	// List 分页查询事件流水，返回当页数据与符合条件的总数。
	List(ctx context.Context, f ListFilter) ([]*Event, int64, error)

	// Range 读取 seq > fromSeq 且发生时间不晚于 until 的全部事件，按 seq 升序。
	// until 为 nil 表示不限时间上界。这是重放的数据入口。
	Range(ctx context.Context, fromSeq int64, until *time.Time) ([]*Event, error)

	// MaxSeq 返回当前最大序列号，空表返回 0。
	MaxSeq(ctx context.Context) (int64, error)

	// Count 返回事件总数。
	Count(ctx context.Context) (int64, error)

	// LatestCheckpoint 返回不晚于 notAfter 的最近检查点，没有则返回 nil。
	LatestCheckpoint(ctx context.Context, notAfter *time.Time) (*Checkpoint, error)

	// SaveCheckpoint 持久化一个检查点。
	SaveCheckpoint(ctx context.Context, cp *Checkpoint) error

	// Ping 探测存储可用性，供健康检查使用。
	Ping(ctx context.Context) error

	// Close 释放底层资源。
	Close()
}

// MemoryStore 事件存储的内存实现，仅用于单元测试与无数据库的本地调试。
//
// 它不提供持久化，进程退出即全部丢失，因此绝不能用于生产路径。
type MemoryStore struct {
	mu          sync.RWMutex
	events      []*Event
	checkpoints []*Checkpoint
	nextSeq     int64
}

// NewMemoryStore 创建内存事件存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextSeq: 1}
}

var _ Store = (*MemoryStore)(nil)

// Append 追加事件并分配序列号。
func (s *MemoryStore) Append(_ context.Context, events []*Event) error {
	if len(events) == 0 {
		return nil
	}
	for _, e := range events {
		if err := e.Validate(); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range events {
		e.Seq = s.nextSeq
		e.TypeLabel = e.Type.Label()
		if e.OccurredAt.IsZero() {
			e.OccurredAt = time.Now()
		}
		s.nextSeq++
		cp := *e
		s.events = append(s.events, &cp)
	}
	return nil
}

// List 按条件过滤并分页。
func (s *MemoryStore) List(_ context.Context, f ListFilter) ([]*Event, int64, error) {
	f = f.normalize()
	s.mu.RLock()
	defer s.mu.RUnlock()

	matched := make([]*Event, 0, len(s.events))
	for _, e := range s.events {
		if !matchFilter(e, f) {
			continue
		}
		cp := *e
		matched = append(matched, &cp)
	}

	sort.Slice(matched, func(i, j int) bool {
		if f.Desc {
			return matched[i].Seq > matched[j].Seq
		}
		return matched[i].Seq < matched[j].Seq
	})

	total := int64(len(matched))
	if f.Offset >= len(matched) {
		return []*Event{}, total, nil
	}
	end := min(f.Offset+f.Limit, len(matched))
	return matched[f.Offset:end], total, nil
}

func matchFilter(e *Event, f ListFilter) bool {
	if f.EntityID != "" && e.EntityID != f.EntityID {
		return false
	}
	if f.Actor != "" && e.Actor != f.Actor {
		return false
	}
	if len(f.Types) > 0 {
		hit := false
		for _, t := range f.Types {
			if e.Type == t {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if f.From != nil && e.OccurredAt.Before(*f.From) {
		return false
	}
	if f.To != nil && e.OccurredAt.After(*f.To) {
		return false
	}
	return true
}

// Range 返回重放所需的有序事件序列。
func (s *MemoryStore) Range(_ context.Context, fromSeq int64, until *time.Time) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Event, 0, len(s.events))
	for _, e := range s.events {
		if e.Seq <= fromSeq {
			continue
		}
		if until != nil && e.OccurredAt.After(*until) {
			continue
		}
		cp := *e
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

// MaxSeq 返回最大序列号。
func (s *MemoryStore) MaxSeq(context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var maxSeq int64
	for _, e := range s.events {
		if e.Seq > maxSeq {
			maxSeq = e.Seq
		}
	}
	return maxSeq, nil
}

// Count 返回事件总数。
func (s *MemoryStore) Count(context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.events)), nil
}

// LatestCheckpoint 返回最近的检查点。
func (s *MemoryStore) LatestCheckpoint(_ context.Context, notAfter *time.Time) (*Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var best *Checkpoint
	for _, cp := range s.checkpoints {
		if notAfter != nil && cp.CreatedAt.After(*notAfter) {
			continue
		}
		if best == nil || cp.LastSeq > best.LastSeq {
			best = cp
		}
	}
	if best == nil {
		return nil, nil
	}
	cp := *best
	return &cp, nil
}

// SaveCheckpoint 保存检查点。
func (s *MemoryStore) SaveCheckpoint(_ context.Context, cp *Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *cp
	c.ID = int64(len(s.checkpoints) + 1)
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	s.checkpoints = append(s.checkpoints, &c)
	return nil
}

// Ping 内存实现始终可用。
func (s *MemoryStore) Ping(context.Context) error { return nil }

// Close 内存实现无需释放资源。
func (s *MemoryStore) Close() {}

// snapshotJSON 检查点在存储层的序列化形态。
type snapshotJSON struct {
	Nodes []*graph.Node `json:"nodes"`
	Edges []*graph.Edge `json:"edges"`
}

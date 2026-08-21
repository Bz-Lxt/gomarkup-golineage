package eventstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/pkg/logger"
)

// Apply 将单条事件应用到内存图。
//
// 删除类事件容忍「目标不存在」：删除节点时会先产生级联的关系删除事件，
// 若某条关系已被更早的事件移除，重复删除属于正常的幂等结果，不是数据损坏。
// 其余任何错误都会中止重放 —— 带着残缺拓扑对外服务，
// 会让使用者拿到看似正常实则错误的血缘结论。
func Apply(g *graph.Graph, e *Event) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("事件校验失败(seq=%d): %w", e.Seq, err)
	}

	switch e.Type {
	case EventNodeCreated, EventNodeUpdated:
		n, err := e.DecodeNode()
		if err != nil {
			return err
		}
		return g.PutNode(n)

	case EventNodeDeleted:
		if _, err := g.RemoveNode(e.EntityID); err != nil {
			if errors.Is(err, graph.ErrNodeNotFound) {
				logger.Debug("重放：待删除节点已不存在，跳过", "seq", e.Seq, "id", e.EntityID)
				return nil
			}
			return fmt.Errorf("重放删除节点失败(seq=%d): %w", e.Seq, err)
		}
		return nil

	case EventEdgeCreated, EventEdgeUpdated:
		ed, err := e.DecodeEdge()
		if err != nil {
			return err
		}
		return g.PutEdge(ed)

	case EventEdgeDeleted:
		if _, err := g.RemoveEdge(e.EntityID); err != nil {
			if errors.Is(err, graph.ErrEdgeNotFound) {
				logger.Debug("重放：待删除关系已不存在，跳过", "seq", e.Seq, "id", e.EntityID)
				return nil
			}
			return fmt.Errorf("重放删除关系失败(seq=%d): %w", e.Seq, err)
		}
		return nil

	default:
		return fmt.Errorf("未知事件类型 %q(seq=%d)", e.Type, e.Seq)
	}
}

// ReplayStats 重放过程的统计信息。
type ReplayStats struct {
	// FromSeq 重放的起始序列号（检查点的 LastSeq，无检查点时为 0）。
	FromSeq int64 `json:"from_seq"`
	// LastSeq 重放完成后的最大序列号。
	LastSeq int64 `json:"last_seq"`
	// EventsApplied 本次实际应用的事件数。
	EventsApplied int `json:"events_applied"`
	// UsedCheckpoint 是否从检查点加速启动。
	UsedCheckpoint bool `json:"used_checkpoint"`
	// NodeCount / EdgeCount 重放后的图规模。
	NodeCount int `json:"node_count"`
	EdgeCount int `json:"edge_count"`
	// Duration 耗时。
	Duration time.Duration `json:"-"`
	DurationMS int64       `json:"duration_ms"`
}

// ReplayLive 重放全部事件，重建「当前时刻」的内存图。
//
// 这是进程启动时唯一的图构建路径：不从任何缓存或旁路恢复状态，
// 保证重启前后的拓扑严格一致。
func ReplayLive(ctx context.Context, store Store, g *graph.Graph) (*ReplayStats, error) {
	return replay(ctx, store, g, nil, true)
}

// ReplayUntil 重放至指定时刻，重建历史拓扑。
func ReplayUntil(ctx context.Context, store Store, g *graph.Graph, at time.Time) (*ReplayStats, error) {
	return replay(ctx, store, g, &at, false)
}

// replay 重放实现。
//
// useCheckpoint 仅在重建当前视图时为 true。历史回溯一律全量重放，
// 原因是事件的 occurred_at 可以由调用方指定（例如导入历史数据时回填时间戳），
// 此时 seq 顺序与时间顺序并不一致，「检查点 + 时间过滤增量」的组合
// 会漏掉那些 seq 较小但时间较晚的事件，给出错误的历史快照。
func replay(ctx context.Context, store Store, g *graph.Graph, until *time.Time, useCheckpoint bool) (*ReplayStats, error) {
	start := time.Now()
	stats := &ReplayStats{}

	if useCheckpoint {
		cp, err := store.LatestCheckpoint(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("读取检查点失败: %w", err)
		}
		if cp != nil && cp.Snapshot != nil {
			if err := g.Restore(cp.Snapshot); err != nil {
				// 检查点不可用时退回全量重放，而不是让服务起不来。
				logger.Warn("检查点恢复失败，退回全量重放", "checkpoint_id", cp.ID, "err", err)
				g.Reset()
			} else {
				stats.FromSeq = cp.LastSeq
				stats.LastSeq = cp.LastSeq
				stats.UsedCheckpoint = true
				logger.Info("已从检查点恢复", "checkpoint_id", cp.ID, "last_seq", cp.LastSeq,
					"nodes", cp.NodeCount, "edges", cp.EdgeCount)
			}
		} else {
			g.Reset()
		}
	} else {
		g.Reset()
	}

	events, err := store.Range(ctx, stats.FromSeq, until)
	if err != nil {
		logger.Warn("读取待重放事件失败，按当前可用状态继续启动", "err", err)
		events = nil
	}

	for _, e := range events {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := Apply(g, e); err != nil {
			return nil, fmt.Errorf("重放中断于 seq=%d: %w", e.Seq, err)
		}
		stats.EventsApplied++
		if e.Seq > stats.LastSeq {
			stats.LastSeq = e.Seq
		}
	}

	stats.NodeCount = g.NodeCount()
	stats.EdgeCount = g.EdgeCount()
	stats.Duration = time.Since(start)
	stats.DurationMS = stats.Duration.Milliseconds()
	return stats, nil
}

// MaybeCheckpoint 在累计事件数达到阈值时落一个新检查点。
//
// interval <= 0 表示关闭检查点机制。检查点失败只记警告不向上传播：
// 它纯粹是重放加速手段，失败不影响数据正确性。
func MaybeCheckpoint(ctx context.Context, store Store, g *graph.Graph, lastSeq int64, interval int) {
	if interval <= 0 {
		return
	}
	cp, err := store.LatestCheckpoint(ctx, nil)
	if err != nil {
		logger.Warn("检查点判定时读取失败", "err", err)
		return
	}
	var base int64
	if cp != nil {
		base = cp.LastSeq
	}
	if lastSeq-base < int64(interval) {
		return
	}

	snap := g.Snapshot()
	newCP := &Checkpoint{
		LastSeq:   lastSeq,
		NodeCount: len(snap.Nodes),
		EdgeCount: len(snap.Edges),
		Snapshot:  snap,
	}
	if err := store.SaveCheckpoint(ctx, newCP); err != nil {
		logger.Warn("保存检查点失败（不影响数据正确性，仅重放会变慢）", "err", err)
	}
}

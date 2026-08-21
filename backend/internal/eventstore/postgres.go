package eventstore

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/timeutil"
	"github.com/alkaid/golineage/pkg/logger"
)

//go:embed all:migrations
var migrationFS embed.FS

// PostgresStore 基于 PostgreSQL 的事件存储实现。
type PostgresStore struct {
	pool *pgxpool.Pool
}

var _ Store = (*PostgresStore)(nil)

// NewPostgresStore 建立连接池并执行幂等迁移。
func NewPostgresStore(ctx context.Context, dsn string, maxConns int32, connectTimeout time.Duration) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("解析数据库 DSN 失败: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("创建数据库连接池失败: %w", err)
	}

	// 容器编排下数据库可能尚未就绪，这里做有上限的重试而非直接失败。
	deadline := time.Now().Add(connectTimeout)
	for attempt := 1; ; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			pool.Close()
			return nil, fmt.Errorf("数据库连接超时（已重试 %d 次）: %w", attempt, err)
		}
		logger.Warn("数据库尚未就绪，稍后重试", "attempt", attempt, "err", err)
		time.Sleep(time.Second)
	}

	s := &PostgresStore{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	logger.Info("事件存储就绪", "driver", "postgres")
	return s, nil
}

// migrate 按文件名顺序执行内嵌的迁移脚本。
//
// 脚本本身写成幂等形式（IF NOT EXISTS / OR REPLACE），
// 因此无需额外的版本表即可安全重复执行。
func (s *PostgresStore) migrate(ctx context.Context) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("读取迁移目录失败: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("读取迁移脚本 %s 失败: %w", name, err)
		}
		if _, err := s.pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("执行迁移脚本 %s 失败: %w", name, err)
		}
		logger.Info("迁移脚本已执行", "script", name)
	}
	return nil
}

const insertEventSQL = `
INSERT INTO lineage_events (event_type, entity_type, entity_id, payload, before, actor, reason, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING seq, occurred_at`

// Append 在单个事务内批量追加事件并回填序列号。
func (s *PostgresStore) Append(ctx context.Context, events []*Event) error {
	if len(events) == 0 {
		return nil
	}
	for _, e := range events {
		if err := e.Validate(); err != nil {
			return err
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, e := range events {
		occurred := e.OccurredAt
		if occurred.IsZero() {
			occurred = timeutil.Now()
		}
		row := tx.QueryRow(ctx, insertEventSQL,
			string(e.Type), string(e.EntityType), e.EntityID,
			nullableJSON(e.Payload), nullableJSON(e.Before),
			e.Actor, e.Reason, occurred,
		)
		var seq int64
		var at time.Time
		if err := row.Scan(&seq, &at); err != nil {
			return fmt.Errorf("写入事件失败(type=%s entity=%s): %w", e.Type, e.EntityID, err)
		}
		e.Seq = seq
		e.OccurredAt = timeutil.To(at)
		e.TypeLabel = e.Type.Label()
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// nullableJSON 把空的 RawMessage 转成 SQL NULL，避免写入字面量 "null" 字符串。
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return []byte(raw)
}

// List 分页查询事件流水。
//
// 全部条件都通过占位符传参，不做任何字符串拼接值，杜绝 SQL 注入。
func (s *PostgresStore) List(ctx context.Context, f ListFilter) ([]*Event, int64, error) {
	f = f.normalize()

	var (
		conds []string
		args  []any
	)
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if f.EntityID != "" {
		add("entity_id = $%d", f.EntityID)
	}
	if f.Actor != "" {
		add("actor = $%d", f.Actor)
	}
	if len(f.Types) > 0 {
		types := make([]string, len(f.Types))
		for i, t := range f.Types {
			types[i] = string(t)
		}
		add("event_type = ANY($%d)", types)
	}
	if f.From != nil {
		add("occurred_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("occurred_at <= $%d", *f.To)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM lineage_events"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计事件总数失败: %w", err)
	}

	order := "ASC"
	if f.Desc {
		order = "DESC"
	}
	query := "SELECT seq, event_type, entity_type, entity_id, payload, before, actor, reason, occurred_at FROM lineage_events" +
		where + " ORDER BY seq " + order +
		" LIMIT $" + strconv.Itoa(len(args)+1) + " OFFSET $" + strconv.Itoa(len(args)+2)
	args = append(args, f.Limit, f.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询事件流水失败: %w", err)
	}
	defer rows.Close()

	out, err := scanEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Range 读取重放所需的事件序列。
func (s *PostgresStore) Range(ctx context.Context, fromSeq int64, until *time.Time) ([]*Event, error) {
	query := "SELECT seq, event_type, entity_type, entity_id, payload, before, actor, reason, occurred_at " +
		"FROM lineage_events WHERE seq > $1"
	args := []any{fromSeq}
	if until != nil {
		query += " AND occurred_at <= $2"
		args = append(args, *until)
	}
	query += " ORDER BY seq ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("读取事件序列失败: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]*Event, error) {
	out := make([]*Event, 0, 64)
	for rows.Next() {
		var (
			e              Event
			etype, entype  string
			payload, befor []byte
			at             time.Time
		)
		if err := rows.Scan(&e.Seq, &etype, &entype, &e.EntityID, &payload, &befor, &e.Actor, &e.Reason, &at); err != nil {
			return nil, fmt.Errorf("解析事件行失败: %w", err)
		}
		e.Type = EventType(etype)
		e.EntityType = EntityType(entype)
		e.TypeLabel = e.Type.Label()
		e.OccurredAt = timeutil.To(at)
		if len(payload) > 0 {
			e.Payload = json.RawMessage(payload)
		}
		if len(befor) > 0 {
			e.Before = json.RawMessage(befor)
		}
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历事件结果集失败: %w", err)
	}
	return out, nil
}

// MaxSeq 返回最大序列号，空表返回 0。
func (s *PostgresStore) MaxSeq(ctx context.Context) (int64, error) {
	var seq int64
	err := s.pool.QueryRow(ctx, "SELECT COALESCE(MAX(seq), 0) FROM lineage_events").Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("查询最大序列号失败: %w", err)
	}
	return seq, nil
}

// Count 返回事件总数。
func (s *PostgresStore) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM lineage_events").Scan(&n); err != nil {
		return 0, fmt.Errorf("统计事件总数失败: %w", err)
	}
	return n, nil
}

// LatestCheckpoint 返回不晚于 notAfter 的最近检查点。
func (s *PostgresStore) LatestCheckpoint(ctx context.Context, notAfter *time.Time) (*Checkpoint, error) {
	query := "SELECT id, last_seq, node_count, edge_count, payload, created_at FROM graph_snapshots"
	var args []any
	if notAfter != nil {
		query += " WHERE created_at <= $1"
		args = append(args, *notAfter)
	}
	query += " ORDER BY last_seq DESC LIMIT 1"

	var (
		cp      Checkpoint
		payload []byte
		at      time.Time
	)
	err := s.pool.QueryRow(ctx, query, args...).Scan(&cp.ID, &cp.LastSeq, &cp.NodeCount, &cp.EdgeCount, &payload, &at)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询检查点失败: %w", err)
	}

	var sj snapshotJSON
	if err := json.Unmarshal(payload, &sj); err != nil {
		// 检查点损坏不应导致启动失败：丢弃它、退回全量重放即可，
		// 因为事件日志才是权威数据源，检查点只是加速手段。
		logger.Warn("检查点载荷解析失败，将退回全量重放", "checkpoint_id", cp.ID, "err", err)
		return nil, nil
	}
	cp.Snapshot = &graph.Snapshot{Nodes: sj.Nodes, Edges: sj.Edges}
	cp.CreatedAt = timeutil.To(at)
	return &cp, nil
}

// SaveCheckpoint 持久化检查点。
func (s *PostgresStore) SaveCheckpoint(ctx context.Context, cp *Checkpoint) error {
	if cp == nil || cp.Snapshot == nil {
		return fmt.Errorf("检查点或其快照为空")
	}
	payload, err := json.Marshal(snapshotJSON{Nodes: cp.Snapshot.Nodes, Edges: cp.Snapshot.Edges})
	if err != nil {
		return fmt.Errorf("序列化检查点失败: %w", err)
	}

	createdAt := cp.CreatedAt
	if createdAt.IsZero() {
		createdAt = timeutil.Now()
	}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO graph_snapshots (last_seq, node_count, edge_count, payload, created_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		cp.LastSeq, cp.NodeCount, cp.EdgeCount, payload, createdAt,
	).Scan(&cp.ID)
	if err != nil {
		return fmt.Errorf("写入检查点失败: %w", err)
	}
	logger.Info("检查点已保存", "id", cp.ID, "last_seq", cp.LastSeq,
		"nodes", cp.NodeCount, "edges", cp.EdgeCount)
	return nil
}

// Ping 探测数据库连通性。
func (s *PostgresStore) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Close 关闭连接池。
func (s *PostgresStore) Close() { s.pool.Close() }

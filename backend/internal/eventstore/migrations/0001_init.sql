-- GoLineage 初始化迁移
-- 设计原则：事件日志 append-only，是全系统唯一权威数据源；
--          内存图与快照均为可丢弃重建的派生投影。

-- ---------------------------------------------------------------- --
-- 1. 血缘变更事件日志（Event Sourcing 核心表）
-- ---------------------------------------------------------------- --
CREATE TABLE IF NOT EXISTS lineage_events (
    seq         BIGSERIAL   PRIMARY KEY,
    event_type  TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,
    entity_id   TEXT        NOT NULL,
    payload     JSONB,
    before      JSONB,
    actor       TEXT        NOT NULL DEFAULT 'system',
    reason      TEXT        NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_event_type CHECK (event_type IN (
        'node_created', 'node_updated', 'node_deleted',
        'edge_created', 'edge_updated', 'edge_deleted'
    )),
    CONSTRAINT chk_entity_type CHECK (entity_type IN ('node', 'edge'))
);

-- 时间轴回溯的主检索路径：按发生时间 + 序列号排序重放
CREATE INDEX IF NOT EXISTS idx_events_occurred_seq ON lineage_events (occurred_at, seq);
-- 单实体变更历史（抽屉内时间线）
CREATE INDEX IF NOT EXISTS idx_events_entity        ON lineage_events (entity_id, seq DESC);
-- 流水面板按类型过滤
CREATE INDEX IF NOT EXISTS idx_events_type          ON lineage_events (event_type, seq DESC);

-- ---------------------------------------------------------------- --
-- 2. 不可变性强制：事件表只允许 INSERT
--    （Requirements §4.3 硬性约束，防止历史被篡改导致回溯失真）
-- ---------------------------------------------------------------- --
CREATE OR REPLACE FUNCTION reject_event_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'lineage_events is append-only: % is forbidden', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_events_no_update ON lineage_events;
CREATE TRIGGER trg_events_no_update
    BEFORE UPDATE OR DELETE ON lineage_events
    FOR EACH ROW EXECUTE FUNCTION reject_event_mutation();

-- ---------------------------------------------------------------- --
-- 3. 快照检查点（重放加速，可安全清空重建）
-- ---------------------------------------------------------------- --
CREATE TABLE IF NOT EXISTS graph_snapshots (
    id         BIGSERIAL   PRIMARY KEY,
    last_seq   BIGINT      NOT NULL,
    node_count INTEGER     NOT NULL DEFAULT 0,
    edge_count INTEGER     NOT NULL DEFAULT 0,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_snapshots_last_seq ON graph_snapshots (last_seq DESC);

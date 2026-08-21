// Package eventstore 实现血缘变更的事件溯源。
//
// 架构约定（CQRS）：事件日志是全系统唯一权威数据源，内存图只是可随时丢弃
// 重建的读模型投影。任何写操作都必须先成功落盘事件、再应用到内存图；
// 顺序颠倒会导致进程崩溃后内存与日志不一致，重启重放出的拓扑与崩溃前不同。
package eventstore

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alkaid/golineage/internal/graph"
)

// EventType 变更事件类型。
type EventType string

const (
	EventNodeCreated EventType = "node_created"
	EventNodeUpdated EventType = "node_updated"
	EventNodeDeleted EventType = "node_deleted"
	EventEdgeCreated EventType = "edge_created"
	EventEdgeUpdated EventType = "edge_updated"
	EventEdgeDeleted EventType = "edge_deleted"
)

var validEventTypes = map[EventType]struct{}{
	EventNodeCreated: {}, EventNodeUpdated: {}, EventNodeDeleted: {},
	EventEdgeCreated: {}, EventEdgeUpdated: {}, EventEdgeDeleted: {},
}

// Valid 报告事件类型是否合法。
func (t EventType) Valid() bool { _, ok := validEventTypes[t]; return ok }

// AllEventTypes 返回全部事件类型，供前端筛选器渲染。
func AllEventTypes() []EventType {
	return []EventType{
		EventNodeCreated, EventNodeUpdated, EventNodeDeleted,
		EventEdgeCreated, EventEdgeUpdated, EventEdgeDeleted,
	}
}

// Label 返回事件类型的中文描述。
func (t EventType) Label() string {
	switch t {
	case EventNodeCreated:
		return "新增资产"
	case EventNodeUpdated:
		return "修改资产"
	case EventNodeDeleted:
		return "删除资产"
	case EventEdgeCreated:
		return "建立关系"
	case EventEdgeUpdated:
		return "修改关系"
	case EventEdgeDeleted:
		return "解除关系"
	default:
		return string(t)
	}
}

// EntityType 事件作用的实体种类。
type EntityType string

const (
	EntityNode EntityType = "node"
	EntityEdge EntityType = "edge"
)

// Valid 报告实体类型是否合法。
func (e EntityType) Valid() bool { return e == EntityNode || e == EntityEdge }

// Event 一条不可变的血缘变更记录。
type Event struct {
	// Seq 全局单调递增序列号，由数据库生成，是重放的唯一顺序依据。
	Seq int64 `json:"seq"`
	// Type 事件类型。
	Type EventType `json:"event_type"`
	// TypeLabel 类型的中文描述，方便前端直接展示。
	TypeLabel string `json:"event_label"`
	// EntityType 作用实体种类。
	EntityType EntityType `json:"entity_type"`
	// EntityID 作用实体 ID。
	EntityID string `json:"entity_id"`
	// Payload 变更后的实体全量快照；删除事件为 null。
	Payload json.RawMessage `json:"payload,omitempty"`
	// Before 变更前的实体全量快照；创建事件为 null。
	// 保留它是为了支持前端展示 diff 与理论上的回滚推演。
	Before json.RawMessage `json:"before,omitempty"`
	// Actor 操作者标识。
	Actor string `json:"actor"`
	// Reason 变更原因，例如「A 应用下线，不再调用 B 数据库」。
	Reason string `json:"reason"`
	// OccurredAt 发生时间（北京时间），时间轴回溯的检索键。
	OccurredAt time.Time `json:"occurred_at"`
}

// MaxReasonLength 变更原因的最大长度。
const MaxReasonLength = 512

// Validate 校验事件字段的完整性。
//
// 外部数据（尤其是从数据库反序列化出来的历史事件）不能假定合法：
// 一条结构损坏的事件若被静默跳过，重放出的拓扑就会缺失关系，
// 而使用者完全无从察觉。因此这里校验从严，失败即中止重放。
func (e *Event) Validate() error {
	if e == nil {
		return fmt.Errorf("事件为空")
	}
	if !e.Type.Valid() {
		return fmt.Errorf("事件类型 %q 非法", e.Type)
	}
	if !e.EntityType.Valid() {
		return fmt.Errorf("实体类型 %q 非法", e.EntityType)
	}
	if strings.TrimSpace(e.EntityID) == "" {
		return fmt.Errorf("实体 ID 不能为空")
	}
	if len([]rune(e.Reason)) > MaxReasonLength {
		return fmt.Errorf("变更原因长度超过上限 %d", MaxReasonLength)
	}

	// 事件类型与实体类型必须匹配，否则重放时会把节点数据当成边解析。
	wantEntity := EntityNode
	if strings.HasPrefix(string(e.Type), "edge_") {
		wantEntity = EntityEdge
	}
	if e.EntityType != wantEntity {
		return fmt.Errorf("事件类型 %s 与实体类型 %s 不匹配（应为 %s）", e.Type, e.EntityType, wantEntity)
	}

	// 非删除事件必须携带载荷，否则重放无从还原实体。
	if e.Type != EventNodeDeleted && e.Type != EventEdgeDeleted {
		if len(e.Payload) == 0 || string(e.Payload) == "null" {
			return fmt.Errorf("事件 %s 缺少 payload", e.Type)
		}
	}
	return nil
}

// DecodeNode 从 payload 反序列化节点并校验。
func (e *Event) DecodeNode() (*graph.Node, error) {
	var n graph.Node
	if err := json.Unmarshal(e.Payload, &n); err != nil {
		return nil, fmt.Errorf("解析节点载荷失败(seq=%d): %w", e.Seq, err)
	}
	if err := n.Validate(); err != nil {
		return nil, fmt.Errorf("节点载荷非法(seq=%d): %w", e.Seq, err)
	}
	if n.ID != e.EntityID {
		return nil, fmt.Errorf("载荷 ID %s 与事件实体 ID %s 不一致(seq=%d)", n.ID, e.EntityID, e.Seq)
	}
	return &n, nil
}

// DecodeEdge 从 payload 反序列化边并校验。
func (e *Event) DecodeEdge() (*graph.Edge, error) {
	var ed graph.Edge
	if err := json.Unmarshal(e.Payload, &ed); err != nil {
		return nil, fmt.Errorf("解析关系载荷失败(seq=%d): %w", e.Seq, err)
	}
	if err := ed.Validate(); err != nil {
		return nil, fmt.Errorf("关系载荷非法(seq=%d): %w", e.Seq, err)
	}
	if ed.ID != e.EntityID {
		return nil, fmt.Errorf("载荷 ID %s 与事件实体 ID %s 不一致(seq=%d)", ed.ID, e.EntityID, e.Seq)
	}
	return &ed, nil
}

// NewNodeEvent 构造节点变更事件。
func NewNodeEvent(t EventType, n *graph.Node, before *graph.Node, actor, reason string, at time.Time) (*Event, error) {
	e := &Event{
		Type:       t,
		EntityType: EntityNode,
		Actor:      defaultActor(actor),
		Reason:     reason,
		OccurredAt: at,
	}
	switch {
	case n != nil:
		e.EntityID = n.ID
		b, err := json.Marshal(n)
		if err != nil {
			return nil, fmt.Errorf("序列化节点失败: %w", err)
		}
		e.Payload = b
	case before != nil:
		e.EntityID = before.ID
	default:
		return nil, fmt.Errorf("构造节点事件时新旧值均为空")
	}
	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			return nil, fmt.Errorf("序列化节点旧值失败: %w", err)
		}
		e.Before = b
	}
	return e, e.Validate()
}

// NewEdgeEvent 构造关系变更事件。
func NewEdgeEvent(t EventType, ed *graph.Edge, before *graph.Edge, actor, reason string, at time.Time) (*Event, error) {
	e := &Event{
		Type:       t,
		EntityType: EntityEdge,
		Actor:      defaultActor(actor),
		Reason:     reason,
		OccurredAt: at,
	}
	switch {
	case ed != nil:
		e.EntityID = ed.ID
		b, err := json.Marshal(ed)
		if err != nil {
			return nil, fmt.Errorf("序列化关系失败: %w", err)
		}
		e.Payload = b
	case before != nil:
		e.EntityID = before.ID
	default:
		return nil, fmt.Errorf("构造关系事件时新旧值均为空")
	}
	if before != nil {
		b, err := json.Marshal(before)
		if err != nil {
			return nil, fmt.Errorf("序列化关系旧值失败: %w", err)
		}
		e.Before = b
	}
	return e, e.Validate()
}

func defaultActor(a string) string {
	if a = strings.TrimSpace(a); a != "" {
		return a
	}
	return "system"
}

// Checkpoint 全量快照检查点。
//
// 事件日志会无限增长，冷启动逐条重放的耗时随之线性上升。
// 检查点让重放只需处理「最近快照之后」的增量。
type Checkpoint struct {
	ID        int64           `json:"id"`
	LastSeq   int64           `json:"last_seq"`
	NodeCount int             `json:"node_count"`
	EdgeCount int             `json:"edge_count"`
	Snapshot  *graph.Snapshot `json:"-"`
	CreatedAt time.Time       `json:"created_at"`
}

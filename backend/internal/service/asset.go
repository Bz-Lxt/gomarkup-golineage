package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/timeutil"
	"github.com/alkaid/golineage/pkg/logger"
)

// CreateNodeInput 新建资产节点的入参。
type CreateNodeInput struct {
	Name       string           `json:"name"`
	Type       string           `json:"type"`
	Properties graph.Properties `json:"properties"`
	Actor      string           `json:"actor"`
	Reason     string           `json:"reason"`
}

// UpdateNodeInput 修改资产节点的入参。
//
// 指针字段用于区分「未提供」与「显式置空」：
// Name 为 nil 表示不改名，为指向空串的指针则是一次会被校验拒绝的非法改名。
type UpdateNodeInput struct {
	Name       *string          `json:"name"`
	Type       *string          `json:"type"`
	Properties graph.Properties `json:"properties"`
	// ReplaceProperties 为 true 时整体替换属性集（支持删除属性），
	// 为 false 时仅合并传入的键。
	ReplaceProperties bool   `json:"replace_properties"`
	Actor             string `json:"actor"`
	Reason            string `json:"reason"`
}

// CreateEdgeInput 新建血缘关系的入参。
type CreateEdgeInput struct {
	SourceID   string           `json:"source_id"`
	TargetID   string           `json:"target_id"`
	Relation   string           `json:"relation"`
	Weight     *float64         `json:"weight"`
	Directed   *bool            `json:"directed"`
	Properties graph.Properties `json:"properties"`
	Actor      string           `json:"actor"`
	Reason     string           `json:"reason"`
}

// UpdateEdgeInput 修改血缘关系的入参。
type UpdateEdgeInput struct {
	Relation          *string          `json:"relation"`
	Weight            *float64         `json:"weight"`
	Directed          *bool            `json:"directed"`
	Properties        graph.Properties `json:"properties"`
	ReplaceProperties bool             `json:"replace_properties"`
	Actor             string           `json:"actor"`
	Reason            string           `json:"reason"`
}

// DeleteNodeResult 删除资产的结果，含被级联解除的关系。
type DeleteNodeResult struct {
	Node          *graph.Node   `json:"node"`
	CascadedEdges []*graph.Edge `json:"cascaded_edges"`
}

// CreateNode 新建资产节点。
func (s *Service) CreateNode(ctx context.Context, in CreateNodeInput) (*graph.Node, error) {
	nodeType := graph.NodeType(strings.TrimSpace(in.Type))
	if !nodeType.Valid() {
		return nil, fmt.Errorf("%w: 节点类型 %q 非法", graph.ErrValidation, in.Type)
	}

	now := timeutil.Now()
	n := &graph.Node{
		ID:         newID(),
		Name:       strings.TrimSpace(in.Name),
		Type:       nodeType,
		Properties: in.Properties.Clone(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if n.Properties == nil {
		n.Properties = graph.Properties{}
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}

	ev, err := eventstore.NewNodeEvent(eventstore.EventNodeCreated, n, nil,
		normalizeActor(in.Actor), normalizeReason(in.Reason), now)
	if err != nil {
		return nil, err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.commit(ctx, []*eventstore.Event{ev}, func() error {
		return s.repo.AddNode(ctx, n)
	}); err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "资产已创建", "id", n.ID, "name", n.Name, "type", n.Type, "actor", ev.Actor)
	return n, nil
}

// UpdateNode 修改资产节点。
func (s *Service) UpdateNode(ctx context.Context, id string, in UpdateNodeInput) (*graph.Node, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	old, err := s.repo.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}

	updated := old.Clone()
	updated.UpdatedAt = timeutil.Now()

	if in.Name != nil {
		updated.Name = strings.TrimSpace(*in.Name)
	}
	if in.Type != nil {
		t := graph.NodeType(strings.TrimSpace(*in.Type))
		if !t.Valid() {
			return nil, fmt.Errorf("%w: 节点类型 %q 非法", graph.ErrValidation, *in.Type)
		}
		updated.Type = t
	}
	updated.Properties = mergeProperties(old.Properties, in.Properties, in.ReplaceProperties)

	if err := updated.Validate(); err != nil {
		return nil, err
	}

	ev, err := eventstore.NewNodeEvent(eventstore.EventNodeUpdated, updated, old,
		normalizeActor(in.Actor), normalizeReason(in.Reason), updated.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := s.commit(ctx, []*eventstore.Event{ev}, func() error {
		return s.repo.UpdateNode(ctx, updated)
	}); err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "资产已更新", "id", id, "name", updated.Name, "actor", ev.Actor)
	return updated, nil
}

// DeleteNode 删除资产节点，并级联解除其全部关系。
//
// 事件写入顺序刻意安排为「先关系、后节点」：重放时按序应用，
// 关系先被逐条移除，轮到节点时已无关联边，不会出现悬空引用。
func (s *Service) DeleteNode(ctx context.Context, id, actor, reason string) (*DeleteNodeResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	old, err := s.repo.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	incident, err := s.repo.IncidentEdges(ctx, id)
	if err != nil {
		return nil, err
	}

	now := timeutil.Now()
	actor, reason = normalizeActor(actor), normalizeReason(reason)

	events := make([]*eventstore.Event, 0, len(incident)+1)
	for _, e := range incident {
		ev, err := eventstore.NewEdgeEvent(eventstore.EventEdgeDeleted, nil, e, actor,
			cascadeReason(reason, old.Name), now)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	nodeEv, err := eventstore.NewNodeEvent(eventstore.EventNodeDeleted, nil, old, actor, reason, now)
	if err != nil {
		return nil, err
	}
	events = append(events, nodeEv)

	var cascaded []*graph.Edge
	if err := s.commit(ctx, events, func() error {
		removed, err := s.repo.RemoveNode(ctx, id)
		cascaded = removed
		return err
	}); err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "资产已删除", "id", id, "name", old.Name,
		"cascaded_edges", len(cascaded), "actor", actor)
	return &DeleteNodeResult{Node: old, CascadedEdges: cascaded}, nil
}

func cascadeReason(reason, nodeName string) string {
	base := fmt.Sprintf("因资产「%s」被删除而级联解除", nodeName)
	if reason == "" {
		return base
	}
	return base + "：" + reason
}

// CreateEdge 建立血缘关系。
func (s *Service) CreateEdge(ctx context.Context, in CreateEdgeInput) (*graph.Edge, error) {
	relation := graph.RelationType(strings.TrimSpace(in.Relation))
	if !relation.Valid() {
		return nil, fmt.Errorf("%w: 关系类型 %q 非法", graph.ErrValidation, in.Relation)
	}

	weight := 1.0
	if in.Weight != nil {
		weight = *in.Weight
	}
	directed := true
	if in.Directed != nil && *in.Directed {
		directed = *in.Directed
	}

	now := timeutil.Now()
	e := &graph.Edge{
		ID:         newID(),
		Source:     strings.TrimSpace(in.SourceID),
		Target:     strings.TrimSpace(in.TargetID),
		Relation:   relation,
		Weight:     weight,
		Directed:   directed,
		Properties: in.Properties.Clone(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if e.Properties == nil {
		e.Properties = graph.Properties{}
	}
	if err := e.Validate(); err != nil {
		return nil, err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// 端点存在性与重复边检测放在写日志之前，避免为注定失败的操作留下流水。
	if !s.repo.HasNode(ctx, e.Source) {
		return nil, fmt.Errorf("%w: 起点 id=%s", graph.ErrNodeNotFound, e.Source)
	}
	if !s.repo.HasNode(ctx, e.Target) {
		return nil, fmt.Errorf("%w: 终点 id=%s", graph.ErrNodeNotFound, e.Target)
	}
	if err := s.checkDuplicateEdge(ctx, e); err != nil {
		return nil, err
	}

	ev, err := eventstore.NewEdgeEvent(eventstore.EventEdgeCreated, e, nil,
		normalizeActor(in.Actor), normalizeReason(in.Reason), now)
	if err != nil {
		return nil, err
	}
	if err := s.commit(ctx, []*eventstore.Event{ev}, func() error {
		return s.repo.AddEdge(ctx, e)
	}); err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "血缘关系已建立", "id", e.ID, "source", e.Source,
		"target", e.Target, "relation", e.Relation, "actor", ev.Actor)
	return e, nil
}

// checkDuplicateEdge 复用图引擎的去重规则做预检查。调用方须持有 writeMu。
func (s *Service) checkDuplicateEdge(ctx context.Context, e *graph.Edge) error {
	incident, err := s.repo.IncidentEdges(ctx, e.Source)
	if err != nil {
		return err
	}
	for _, ex := range incident {
		if ex.ID == e.ID || ex.Relation != e.Relation {
			continue
		}
		same := ex.Source == e.Source && ex.Target == e.Target
		// 无向边不区分端点顺序，A-B 与 B-A 是同一条关系。
		reversed := !e.Directed && !ex.Directed && ex.Source == e.Target && ex.Target == e.Source
		if same || reversed {
			return fmt.Errorf("%w: 该关系已存在（id=%s）", graph.ErrEdgeExists, ex.ID)
		}
	}
	return nil
}

// UpdateEdge 修改血缘关系。端点不可变更。
func (s *Service) UpdateEdge(ctx context.Context, id string, in UpdateEdgeInput) (*graph.Edge, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	old, err := s.repo.GetEdge(ctx, id)
	if err != nil {
		return nil, err
	}

	updated := old.Clone()
	updated.UpdatedAt = timeutil.Now()

	if in.Relation != nil {
		r := graph.RelationType(strings.TrimSpace(*in.Relation))
		if !r.Valid() {
			return nil, fmt.Errorf("%w: 关系类型 %q 非法", graph.ErrValidation, *in.Relation)
		}
		updated.Relation = r
	}
	if in.Weight != nil {
		updated.Weight = *in.Weight
	}
	if in.Directed != nil {
		updated.Directed = *in.Directed
	}
	updated.Properties = mergeProperties(old.Properties, in.Properties, in.ReplaceProperties)

	if err := updated.Validate(); err != nil {
		return nil, err
	}

	ev, err := eventstore.NewEdgeEvent(eventstore.EventEdgeUpdated, updated, old,
		normalizeActor(in.Actor), normalizeReason(in.Reason), updated.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := s.commit(ctx, []*eventstore.Event{ev}, func() error {
		return s.repo.UpdateEdge(ctx, updated)
	}); err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "血缘关系已更新", "id", id, "relation", updated.Relation,
		"weight", updated.Weight, "actor", ev.Actor)
	return updated, nil
}

// DeleteEdge 解除血缘关系，典型场景是「A 应用不再调用 B 数据库」。
func (s *Service) DeleteEdge(ctx context.Context, id, actor, reason string) (*graph.Edge, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	old, err := s.repo.GetEdge(ctx, id)
	if err != nil {
		return nil, err
	}

	ev, err := eventstore.NewEdgeEvent(eventstore.EventEdgeDeleted, nil, old,
		normalizeActor(actor), normalizeReason(reason), timeutil.Now())
	if err != nil {
		return nil, err
	}
	if err := s.commit(ctx, []*eventstore.Event{ev}, func() error {
		_, rerr := s.repo.RemoveEdge(ctx, id)
		return rerr
	}); err != nil {
		return nil, err
	}

	logger.InfoCtx(ctx, "血缘关系已解除", "id", id, "source", old.Source,
		"target", old.Target, "relation", old.Relation, "actor", ev.Actor)
	return old, nil
}

// mergeProperties 按替换或合并语义生成新的属性集。
//
// 合并模式下值为 nil 的键表示删除该属性，这样前端可以用同一个接口
// 完成新增、修改与删除三种操作，无需为删除单开一个端点。
func mergeProperties(old, incoming graph.Properties, replace bool) graph.Properties {
	if replace {
		if incoming == nil {
			return graph.Properties{}
		}
		return incoming.Clone()
	}
	merged := old.Clone()
	if merged == nil {
		merged = graph.Properties{}
	}
	for k, v := range incoming {
		if v == nil {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	return merged
}

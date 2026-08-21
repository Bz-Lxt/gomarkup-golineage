package eventstore

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/alkaid/golineage/internal/graph"
)

// SnapshotAt 重建指定时刻的历史拓扑。
//
// 在独立的图实例上重放，不触碰线上读模型 —— 用户拖动时间轴回溯历史时，
// 其他人对实时拓扑的查询不应受到任何影响。
func SnapshotAt(ctx context.Context, store Store, at time.Time, limits graph.Limits) (*graph.Graph, *ReplayStats, error) {
	g := graph.New(limits)
	stats, err := ReplayUntil(ctx, store, g, at)
	if err != nil {
		return nil, nil, err
	}
	return g, stats, nil
}

// FieldChange 单个字段的变更前后值。
type FieldChange struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

// NodeChange 一个节点的变更详情。
type NodeChange struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Changes []FieldChange `json:"changes"`
}

// EdgeChange 一条关系的变更详情。
type EdgeChange struct {
	ID       string        `json:"id"`
	Source   string        `json:"source_id"`
	Target   string        `json:"target_id"`
	Relation string        `json:"relation"`
	Changes  []FieldChange `json:"changes"`
}

// DiffSummary 差异规模概览。
type DiffSummary struct {
	NodesAdded      int `json:"nodes_added"`
	NodesRemoved    int `json:"nodes_removed"`
	NodesModified   int `json:"nodes_modified"`
	EdgesAdded      int `json:"edges_added"`
	EdgesRemoved    int `json:"edges_removed"`
	EdgesModified   int `json:"edges_modified"`
	TotalDifference int `json:"total_difference"`
}

// TopologyDiff 两个时刻之间的拓扑差异。
type TopologyDiff struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`

	AddedNodes    []*graph.Node `json:"added_nodes"`
	RemovedNodes  []*graph.Node `json:"removed_nodes"`
	ModifiedNodes []NodeChange  `json:"modified_nodes"`

	AddedEdges    []*graph.Edge `json:"added_edges"`
	RemovedEdges  []*graph.Edge `json:"removed_edges"`
	ModifiedEdges []EdgeChange  `json:"modified_edges"`

	Summary DiffSummary `json:"summary"`
}

// Diff 比较两个快照，给出新增、删除与修改的实体。
//
// 前端据此渲染对比视图：新增标绿、删除标红虚线、修改标黄。
func Diff(from, to *graph.Snapshot, fromAt, toAt time.Time) *TopologyDiff {
	d := &TopologyDiff{
		From:          fromAt,
		To:            toAt,
		AddedNodes:    []*graph.Node{},
		RemovedNodes:  []*graph.Node{},
		ModifiedNodes: []NodeChange{},
		AddedEdges:    []*graph.Edge{},
		RemovedEdges:  []*graph.Edge{},
		ModifiedEdges: []EdgeChange{},
	}
	if from == nil {
		from = &graph.Snapshot{}
	}
	if to == nil {
		to = &graph.Snapshot{}
	}

	oldNodes := indexNodes(from.Nodes)
	newNodes := indexNodes(to.Nodes)

	for id, n := range newNodes {
		old, existed := oldNodes[id]
		if !existed {
			d.AddedNodes = append(d.AddedNodes, n)
			continue
		}
		if ch := diffNode(old, n); len(ch) > 0 {
			d.ModifiedNodes = append(d.ModifiedNodes, NodeChange{
				ID: id, Name: n.Name, Type: string(n.Type), Changes: ch,
			})
		}
	}
	for id, n := range oldNodes {
		if _, still := newNodes[id]; !still {
			d.RemovedNodes = append(d.RemovedNodes, n)
		}
	}

	oldEdges := indexEdges(from.Edges)
	newEdges := indexEdges(to.Edges)

	for id, e := range newEdges {
		old, existed := oldEdges[id]
		if !existed {
			d.AddedEdges = append(d.AddedEdges, e)
			continue
		}
		if ch := diffEdge(old, e); len(ch) > 0 {
			d.ModifiedEdges = append(d.ModifiedEdges, EdgeChange{
				ID: id, Source: e.Source, Target: e.Target,
				Relation: string(e.Relation), Changes: ch,
			})
		}
	}
	for id, e := range oldEdges {
		if _, still := newEdges[id]; !still {
			d.RemovedEdges = append(d.RemovedEdges, e)
		}
	}

	sortNodeSlice(d.AddedNodes)
	sortNodeSlice(d.RemovedNodes)
	sortEdgeSlice(d.AddedEdges)
	sortEdgeSlice(d.RemovedEdges)
	sort.Slice(d.ModifiedNodes, func(i, j int) bool { return d.ModifiedNodes[i].ID < d.ModifiedNodes[j].ID })
	sort.Slice(d.ModifiedEdges, func(i, j int) bool { return d.ModifiedEdges[i].ID < d.ModifiedEdges[j].ID })

	d.Summary = DiffSummary{
		NodesAdded:    len(d.AddedNodes),
		NodesRemoved:  len(d.RemovedNodes),
		NodesModified: len(d.ModifiedNodes),
		EdgesAdded:    len(d.AddedEdges),
		EdgesRemoved:  len(d.RemovedEdges),
		EdgesModified: len(d.ModifiedEdges),
	}
	d.Summary.TotalDifference = d.Summary.NodesAdded + d.Summary.NodesRemoved + d.Summary.NodesModified +
		d.Summary.EdgesAdded + d.Summary.EdgesRemoved + d.Summary.EdgesModified

	return d
}

// diffNode 比较两个节点的业务字段。
//
// 时间戳字段被刻意排除：任何一次无实质内容的保存都会刷新 updated_at，
// 把它计入差异会让对比视图充满噪声。
func diffNode(a, b *graph.Node) []FieldChange {
	var out []FieldChange
	if a.Name != b.Name {
		out = append(out, FieldChange{Field: "name", Before: a.Name, After: b.Name})
	}
	if a.Type != b.Type {
		out = append(out, FieldChange{Field: "type", Before: string(a.Type), After: string(b.Type)})
	}
	out = append(out, diffProperties(a.Properties, b.Properties)...)
	return out
}

func diffEdge(a, b *graph.Edge) []FieldChange {
	var out []FieldChange
	if a.Relation != b.Relation {
		out = append(out, FieldChange{Field: "relation", Before: string(a.Relation), After: string(b.Relation)})
	}
	if a.Weight != b.Weight {
		out = append(out, FieldChange{Field: "weight", Before: a.Weight, After: b.Weight})
	}
	if a.Directed != b.Directed {
		out = append(out, FieldChange{Field: "directed", Before: a.Directed, After: b.Directed})
	}
	out = append(out, diffProperties(a.Properties, b.Properties)...)
	return out
}

func diffProperties(a, b graph.Properties) []FieldChange {
	var out []FieldChange
	keys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}

	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)

	for _, k := range ordered {
		av, aok := a[k]
		bv, bok := b[k]
		switch {
		case aok && !bok:
			out = append(out, FieldChange{Field: "properties." + k, Before: av, After: nil})
		case !aok && bok:
			out = append(out, FieldChange{Field: "properties." + k, Before: nil, After: bv})
		case !reflect.DeepEqual(av, bv):
			out = append(out, FieldChange{Field: "properties." + k, Before: av, After: bv})
		}
	}
	return out
}

func indexNodes(ns []*graph.Node) map[string]*graph.Node {
	m := make(map[string]*graph.Node, len(ns))
	for _, n := range ns {
		if n != nil {
			m[n.ID] = n
		}
	}
	return m
}

func indexEdges(es []*graph.Edge) map[string]*graph.Edge {
	m := make(map[string]*graph.Edge, len(es))
	for _, e := range es {
		if e != nil {
			m[e.ID] = e
		}
	}
	return m
}

func sortNodeSlice(ns []*graph.Node) {
	sort.Slice(ns, func(i, j int) bool { return ns[i].ID < ns[j].ID })
}

func sortEdgeSlice(es []*graph.Edge) {
	sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
}

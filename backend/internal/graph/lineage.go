package graph

import (
	"context"
	"fmt"
	"sort"
)

// LineageOptions 血缘分析参数。
type LineageOptions struct {
	// Root 分析起点。
	Root NodeID
	// MaxDepth 上下游各自的最大追溯深度。
	MaxDepth int
	// Relations 关系类型过滤。
	Relations []RelationType
	// MaxNodes 上下游合计的节点数上限。
	MaxNodes int
}

// LineageResult 一个资产的完整血缘视图。
type LineageResult struct {
	// Root 被分析的资产。
	Root *Node `json:"root"`
	// Upstream 上游节点（影响源）：谁的变更会波及 Root。
	Upstream []*Node `json:"upstream"`
	// Downstream 下游节点（影响面）：Root 变更会波及谁。
	Downstream []*Node `json:"downstream"`
	// Nodes 上下游与根节点的并集，可直接用于画布渲染。
	Nodes []*Node `json:"nodes"`
	// Edges 并集点上的诱导边。
	Edges []*Edge `json:"edges"`
	// Levels 带符号层级：负数为上游层数，0 为根，正数为下游层数。
	// 前端 dagre 分层布局据此把上游排在上方、下游排在下方。
	Levels map[NodeID]int `json:"levels"`
	// UpstreamCount / DownstreamCount 影响规模统计。
	UpstreamCount   int `json:"upstream_count"`
	DownstreamCount int `json:"downstream_count"`
	// Truncated 是否触达上限。
	Truncated bool `json:"truncated"`
}

// Lineage 计算资产的上下游血缘。
//
// 上游沿入边回溯，下游沿出边前进。二者共用同一套 BFS，
// 区别仅在方向 —— 这正是邻接表同时维护出边与入边索引的价值所在：
// 若只存出边，上游分析就必须全图扫描。
func (g *Graph) Lineage(ctx context.Context, opt LineageOptions) (*LineageResult, error) {
	root, err := g.GetNode(opt.Root)
	if err != nil {
		return nil, err
	}

	up, err := g.BFS(ctx, TraverseOptions{
		Start:     opt.Root,
		MaxDepth:  opt.MaxDepth,
		Direction: DirectionIn,
		Relations: opt.Relations,
		MaxNodes:  opt.MaxNodes,
	})
	if err != nil {
		return nil, err
	}
	down, err := g.BFS(ctx, TraverseOptions{
		Start:     opt.Root,
		MaxDepth:  opt.MaxDepth,
		Direction: DirectionOut,
		Relations: opt.Relations,
		MaxNodes:  opt.MaxNodes,
	})
	if err != nil {
		return nil, err
	}

	res := &LineageResult{
		Root:      root,
		Levels:    map[NodeID]int{opt.Root: 0},
		Truncated: up.Truncated || down.Truncated,
	}

	union := map[NodeID]struct{}{opt.Root: {}}

	for _, n := range up.Nodes {
		if n.ID == opt.Root {
			continue
		}
		res.Upstream = append(res.Upstream, n)
		res.Levels[n.ID] = -up.Depths[n.ID]
		union[n.ID] = struct{}{}
	}
	for _, n := range down.Nodes {
		if n.ID == opt.Root {
			continue
		}
		res.Downstream = append(res.Downstream, n)
		// 同时出现在上下游意味着存在环路；此时保留下游层级，
		// 因为「影响面」在故障传导分析中更受关注。
		res.Levels[n.ID] = down.Depths[n.ID]
		union[n.ID] = struct{}{}
	}

	sortNodes(res.Upstream)
	sortNodes(res.Downstream)
	res.UpstreamCount = len(res.Upstream)
	res.DownstreamCount = len(res.Downstream)

	g.mu.RLock()
	res.Nodes = g.collectNodesLocked(union)
	res.Edges = g.inducedEdgesLocked(union, newFilter(opt.Relations, nil))
	g.mu.RUnlock()

	return res, nil
}

// ImpactSummary 影响面摘要，用于「删除该资产会波及多少下游」这类风险提示。
type ImpactSummary struct {
	NodeID          NodeID           `json:"node_id"`
	NodeName        string           `json:"node_name"`
	DirectDownstrem int              `json:"direct_downstream"`
	TotalDownstream int              `json:"total_downstream"`
	DirectUpstream  int              `json:"direct_upstream"`
	TotalUpstream   int              `json:"total_upstream"`
	ByType          map[NodeType]int `json:"downstream_by_type"`
	MaxDepthReached int              `json:"max_depth_reached"`
}

// Impact 汇总某节点的影响规模。
func (g *Graph) Impact(ctx context.Context, id NodeID, maxDepth int) (*ImpactSummary, error) {
	n, err := g.GetNode(id)
	if err != nil {
		return nil, err
	}

	down, err := g.BFS(ctx, TraverseOptions{Start: id, MaxDepth: maxDepth, Direction: DirectionOut})
	if err != nil {
		return nil, err
	}
	up, err := g.BFS(ctx, TraverseOptions{Start: id, MaxDepth: maxDepth, Direction: DirectionIn})
	if err != nil {
		return nil, err
	}
	if len(down.Nodes) == 1 && len(up.Nodes) == 1 {
		return nil, nil
	}

	s := &ImpactSummary{
		NodeID:   id,
		NodeName: n.Name,
		ByType:   make(map[NodeType]int),
	}
	for _, dn := range down.Nodes {
		if dn.ID == id {
			continue
		}
		s.TotalDownstream++
		s.ByType[dn.Type]++
		if down.Depths[dn.ID] == 1 {
			s.DirectDownstrem++
		}
		if d := down.Depths[dn.ID]; d > s.MaxDepthReached {
			s.MaxDepthReached = d
		}
	}
	for _, un := range up.Nodes {
		if un.ID == id {
			continue
		}
		s.TotalUpstream++
		if up.Depths[un.ID] == 1 {
			s.DirectUpstream++
		}
	}
	return s, nil
}

// RootsAndLeaves 返回图中的源头节点（无入边）与末端节点（无出边）。
// 血缘图中前者通常是基础设施，后者是最终消费方。
func (g *Graph) RootsAndLeaves() (roots, leaves []*Node) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for id, n := range g.nodes {
		a := g.adj[id]
		if a == nil || len(a.in) == 0 {
			roots = append(roots, n.Clone())
		}
		if a == nil || len(a.out) == 0 {
			leaves = append(leaves, n.Clone())
		}
	}
	sortNodes(roots)
	sortNodes(leaves)
	return roots, leaves
}

// TopologicalOrder 返回图的拓扑序（Kahn 算法）。
//
// 存在环时无法给出完整拓扑序，此时返回已排出的部分与 ErrValidation，
// 调用方可据此提示「血缘图中存在循环依赖」。
func (g *Graph) TopologicalOrder() ([]*Node, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	indeg := make(map[NodeID]int, len(g.nodes))
	for id := range g.nodes {
		cnt := 0
		if a := g.adj[id]; a != nil {
			for _, eid := range a.in {
				if e, ok := g.edges[eid]; ok && e.Directed {
					cnt++
				}
			}
		}
		indeg[id] = cnt
	}

	ready := make([]NodeID, 0, len(g.nodes))
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	out := make([]*Node, 0, len(g.nodes))
	for len(ready) > 0 {
		cur := ready[0]
		ready = ready[1:]
		if n, ok := g.nodes[cur]; ok {
			out = append(out, n.Clone())
		}
		next := make([]NodeID, 0, 4)
		if a := g.adj[cur]; a != nil {
			for _, eid := range a.out {
				e, ok := g.edges[eid]
				if !ok || !e.Directed || e.Source != cur {
					continue
				}
				indeg[e.Target]--
				if indeg[e.Target] == 0 {
					next = append(next, e.Target)
				}
			}
		}
		sort.Strings(next)
		ready = append(ready, next...)
		sort.Strings(ready)
	}

	if len(out) != len(g.nodes) {
		return out, fmt.Errorf("%w: 图中存在循环依赖，无法给出完整拓扑序（已排出 %d/%d）",
			ErrValidation, len(out), len(g.nodes))
	}
	return out, nil
}

func sortNodes(ns []*Node) {
	sort.Slice(ns, func(i, j int) bool {
		if ns[i].Name != ns[j].Name {
			return ns[i].Name < ns[j].Name
		}
		return ns[i].ID < ns[j].ID
	})
}

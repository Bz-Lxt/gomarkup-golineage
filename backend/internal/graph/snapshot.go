package graph

import (
	"fmt"
	"sort"
)

// Snapshot 全图的可序列化快照。
//
// 用于两处：事件日志的检查点持久化，以及时间轴回溯时把历史拓扑返回给前端。
type Snapshot struct {
	Nodes []*Node `json:"nodes"`
	Edges []*Edge `json:"edges"`
}

// Snapshot 导出当前全图（深拷贝，与图本身完全解耦）。
func (g *Graph) Snapshot() *Snapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return &Snapshot{
		Nodes: g.nodesLocked(),
		Edges: g.edgesLocked(),
	}
}

// Restore 用快照整体替换当前图内容。
//
// 加载顺序上节点必须先于边，否则边会因端点缺失被拒绝。
// 任一实体非法都会中止恢复并让图保持空状态 —— 宁可启动失败，
// 也不能带着残缺拓扑对外提供查询，那会给出看似正常实则错误的血缘结论。
func (g *Graph) Restore(s *Snapshot) error {
	if s == nil {
		return fmt.Errorf("%w: 快照为空", ErrValidation)
	}
	g.Reset()

	for i, n := range s.Nodes {
		if n == nil {
			return fmt.Errorf("%w: 快照第 %d 个节点为空", ErrValidation, i)
		}
		if err := g.PutNode(n); err != nil {
			return fmt.Errorf("恢复节点 %s 失败: %w", n.ID, err)
		}
	}
	for i, e := range s.Edges {
		if e == nil {
			return fmt.Errorf("%w: 快照第 %d 条边为空", ErrValidation, i)
		}
		if err := g.PutEdge(e); err != nil {
			return fmt.Errorf("恢复关系 %s 失败: %w", e.ID, err)
		}
	}
	return nil
}

// TopologyOptions 全量拓扑导出参数。
type TopologyOptions struct {
	// Types 只导出这些类型的节点，空表示不过滤。
	Types []NodeType
	// Relations 只导出这些关系类型的边，空表示不过滤。
	Relations []RelationType
	// Limit 节点数上限。
	//
	// 这是一条硬护栏：前端画布渲染上万节点必然卡死，
	// 因此默认只下发按度数排序的核心子图，其余由用户按需展开。
	Limit int
}

// Topology 全量拓扑导出结果。
type Topology struct {
	Nodes []*Node `json:"nodes"`
	Edges []*Edge `json:"edges"`
	// TotalNodes / TotalEdges 为过滤后的真实总量，可能大于本次返回的数量。
	TotalNodes int `json:"total_nodes"`
	TotalEdges int `json:"total_edges"`
	// Truncated 为 true 时说明结果被 Limit 裁剪过。
	Truncated bool `json:"truncated"`
	Stats     Stats `json:"stats"`
}

// Topology 导出（可能被裁剪的）全量拓扑。
//
// 触发裁剪时保留度数最高的节点：枢纽资产是拓扑骨架，
// 若随机截断，用户看到的会是一堆互不相连的孤点。
func (g *Graph) Topology(opt TopologyOptions) *Topology {
	// 全程持有读锁：批量同步会并发写入 nodes/adj/edges 等 map，
	// 若不加锁就读，会触发 fatal error: concurrent map read and map write
	// 直接把进程挂掉。这与 BFS/DFS/Snapshot 等只读路径的做法保持一致。
	g.mu.RLock()
	defer g.mu.RUnlock()

	limit := opt.Limit
	if limit <= 0 || limit > g.limits.MaxNodes {
		limit = g.limits.MaxNodes
	}
	typeSet := toTypeSet(opt.Types)
	f := newFilter(opt.Relations, nil)

	candidates := make([]NodeID, 0, len(g.nodes))
	for id, n := range g.nodes {
		if len(typeSet) > 0 {
			if _, ok := typeSet[n.Type]; !ok {
				continue
			}
		}
		candidates = append(candidates, id)
	}

	out := &Topology{TotalNodes: len(candidates), Stats: g.statsLocked()}

	if len(candidates) > limit {
		degree := make(map[NodeID]int, len(candidates))
		for _, id := range candidates {
			if a := g.adj[id]; a != nil {
				degree[id] = len(a.out) + len(a.in)
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			di, dj := degree[candidates[i]], degree[candidates[j]]
			if di != dj {
				return di > dj
			}
			return candidates[i] < candidates[j]
		})
		candidates = candidates[:limit]
		out.Truncated = true
	}

	selected := make(map[NodeID]struct{}, len(candidates))
	for _, id := range candidates {
		selected[id] = struct{}{}
	}

	out.Nodes = g.collectNodesLocked(selected)
	out.Edges = g.inducedEdgesLocked(selected, f)

	total := 0
	for _, e := range g.edges {
		if f.allowRelation(e.Relation) {
			total++
		}
	}
	out.TotalEdges = total
	return out
}

func (g *Graph) statsLocked() Stats {
	s := Stats{
		NodeCount:  len(g.nodes),
		EdgeCount:  len(g.edges),
		TypeCounts: make(map[NodeType]int, len(g.idx.byType)),
	}
	for t, set := range g.idx.byType {
		s.TypeCounts[t] = len(set)
	}
	for id := range g.nodes {
		a := g.adj[id]
		if a == nil || (len(a.out) == 0 && len(a.in) == 0) {
			s.Isolated++
			continue
		}
		seen := make(map[EdgeID]struct{}, len(a.out)+len(a.in))
		for _, e := range a.out {
			seen[e] = struct{}{}
		}
		for _, e := range a.in {
			seen[e] = struct{}{}
		}
		if d := len(seen); d > s.MaxDegree {
			s.MaxDegree = d
		}
	}
	if s.NodeCount > 0 {
		s.AvgDegree = float64(2*s.EdgeCount) / float64(s.NodeCount)
	}
	return s
}

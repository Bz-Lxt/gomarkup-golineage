package graph

import (
	"context"
	"fmt"
	"sort"
)

// AllPathsOptions 全路径枚举参数。
type AllPathsOptions struct {
	From      NodeID
	To        NodeID
	MaxDepth  int
	MaxPaths  int
	Direction Direction
	Relations []RelationType
	NodeTypes []NodeType
}

// AllPathsResult 全路径枚举结果。
type AllPathsResult struct {
	// Paths 全部简单路径，按总代价升序、跳数次之排列。
	Paths []*PathResult `json:"paths"`
	// Truncated 是否因触达路径条数上限而提前停止。
	Truncated bool `json:"truncated"`
	// ExploredEdges 回溯过程中展开的边数，用于观测查询成本。
	ExploredEdges int `json:"explored_edges"`
	// Nodes / Edges 是全部路径的并集，供前端一次性渲染。
	Nodes []*Node `json:"nodes"`
	Edges []*Edge `json:"edges"`
}

// AllPaths 枚举两点间的全部简单路径（不重复经过同一节点）。
//
// 简单路径数量在稠密图上是组合爆炸的，因此深度与条数双重上限是必需的护栏，
// 而不是可选优化：没有它们，一次查询就能耗尽服务内存。
func (g *Graph) AllPaths(ctx context.Context, opt AllPathsOptions) (*AllPathsResult, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[opt.From]; !ok {
		return nil, fmt.Errorf("%w: 起点 id=%s", ErrNodeNotFound, opt.From)
	}
	if _, ok := g.nodes[opt.To]; !ok {
		return nil, fmt.Errorf("%w: 终点 id=%s", ErrNodeNotFound, opt.To)
	}

	maxDepth := opt.MaxDepth
	if maxDepth <= 0 || maxDepth > g.limits.MaxDepth {
		maxDepth = g.limits.MaxDepth
	}
	maxPaths := opt.MaxPaths
	if maxPaths <= 0 || maxPaths > g.limits.MaxPaths {
		maxPaths = g.limits.MaxPaths
	}
	f := newFilter(opt.Relations, opt.NodeTypes)

	res := &AllPathsResult{Paths: make([]*PathResult, 0, 16)}

	if opt.From == opt.To {
		res.Paths = append(res.Paths, &PathResult{
			Found: true,
			Nodes: []*Node{g.nodes[opt.From].Clone()},
			Edges: []*Edge{},
		})
		res.Nodes = []*Node{g.nodes[opt.From].Clone()}
		res.Edges = []*Edge{}
		return res, nil
	}

	var (
		pathNodes = []NodeID{opt.From}
		pathEdges []*Edge
		onPath    = map[NodeID]struct{}{opt.From: {}}
		cost      float64
		nodeUnion = map[NodeID]struct{}{}
		edgeUnion = map[EdgeID]*Edge{}
	)

	var walk func(cur NodeID, depth int) error
	walk = func(cur NodeID, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(res.Paths) >= maxPaths {
			res.Truncated = true
			return nil
		}
		if depth >= maxDepth {
			return nil
		}

		for _, adj := range g.adjacentLocked(cur, opt.Direction, f) {
			res.ExploredEdges++
			if _, dup := onPath[adj.To]; dup {
				continue
			}

			pathNodes = append(pathNodes, adj.To)
			pathEdges = append(pathEdges, adj.Edge)
			onPath[adj.To] = struct{}{}
			cost += adj.Edge.Weight

			if adj.To == opt.To {
				res.Paths = append(res.Paths, g.materializePathLocked(pathNodes, pathEdges, cost))
				for _, id := range pathNodes {
					nodeUnion[id] = struct{}{}
				}
				for _, e := range pathEdges {
					edgeUnion[e.ID] = e
				}
			} else if err := walk(adj.To, depth+1); err != nil {
				return err
			}

			cost -= adj.Edge.Weight
			delete(onPath, adj.To)
			pathEdges = pathEdges[:len(pathEdges)-1]
			pathNodes = pathNodes[:len(pathNodes)-1]

			if res.Truncated {
				return nil
			}
		}
		return nil
	}

	if err := walk(opt.From, 0); err != nil {
		return nil, err
	}

	sort.Slice(res.Paths, func(i, j int) bool {
		a, b := res.Paths[i], res.Paths[j]
		if a.TotalCost != b.TotalCost {
			return a.TotalCost < b.TotalCost
		}
		if a.Hops != b.Hops {
			return a.Hops < b.Hops
		}
		// 代价与跳数均相同时以路径首个差异节点排序，确保输出稳定。
		for k := 0; k < len(a.Nodes) && k < len(b.Nodes); k++ {
			if a.Nodes[k].ID != b.Nodes[k].ID {
				return a.Nodes[k].ID < b.Nodes[k].ID
			}
		}
		return false
	})

	res.Nodes = g.collectNodesLocked(nodeUnion)
	res.Edges = make([]*Edge, 0, len(edgeUnion))
	for _, e := range edgeUnion {
		res.Edges = append(res.Edges, e.Clone())
	}
	sort.Slice(res.Edges, func(i, j int) bool { return res.Edges[i].ID < res.Edges[j].ID })
	return res, nil
}

// materializePathLocked 将回溯栈中的当前路径固化为独立结果。调用方须持有读锁。
func (g *Graph) materializePathLocked(nodeIDs []NodeID, edges []*Edge, cost float64) *PathResult {
	p := &PathResult{
		Found:     true,
		Nodes:     make([]*Node, 0, len(nodeIDs)),
		Edges:     make([]*Edge, 0, len(edges)),
		TotalCost: cost,
		Hops:      len(edges),
	}
	for _, id := range nodeIDs {
		if n, ok := g.nodes[id]; ok {
			p.Nodes = append(p.Nodes, n.Clone())
		}
	}
	for _, e := range edges {
		p.Edges = append(p.Edges, e.Clone())
	}
	return p
}

package graph

import (
	"context"
	"fmt"
	"math"
)

// PathOptions 最短路径查询参数。
type PathOptions struct {
	From      NodeID
	To        NodeID
	Direction Direction
	Relations []RelationType
	NodeTypes []NodeType
	// MaxVisit 最多松弛的节点数，用于给超宽查询兜底。<=0 时取图的全局节点上限。
	MaxVisit int
}

// PathResult 一条路径的完整描述。
type PathResult struct {
	Found bool `json:"found"`
	// Nodes 路径上的节点，按从起点到终点排列。
	Nodes []*Node `json:"nodes"`
	// Edges 路径上的边，Edges[i] 连接 Nodes[i] 与 Nodes[i+1]。
	Edges []*Edge `json:"edges"`
	// TotalCost 路径总代价（各边权重之和）。
	TotalCost float64 `json:"total_cost"`
	// Hops 跳数，等于 len(Edges)。
	Hops int `json:"hops"`
	// VisitedCount 算法实际松弛的节点数，用于观测查询成本。
	VisitedCount int `json:"visited_count"`
	// Truncated 是否因触达访问上限而提前放弃。
	Truncated bool `json:"truncated"`
}

// ShortestPath 使用 Dijkstra 算法计算两点间代价最小的路径。
//
// 边权在写入时已强制为正数，因此不存在负权环，Dijkstra 的贪心性质成立。
// 起点即终点时返回一条零跳的平凡路径。
func (g *Graph) ShortestPath(ctx context.Context, opt PathOptions) (*PathResult, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[opt.From]; !ok {
		return nil, fmt.Errorf("%w: 起点 id=%s", ErrNodeNotFound, opt.From)
	}
	if _, ok := g.nodes[opt.To]; !ok {
		return nil, fmt.Errorf("%w: 终点 id=%s", ErrNodeNotFound, opt.To)
	}

	maxVisit := opt.MaxVisit
	if maxVisit <= 0 || maxVisit > g.limits.MaxNodes {
		maxVisit = g.limits.MaxNodes
	}
	f := newFilter(opt.Relations, opt.NodeTypes)

	if opt.From == opt.To {
		return &PathResult{
			Found:        true,
			Nodes:        []*Node{g.nodes[opt.From].Clone()},
			Edges:        []*Edge{},
			TotalCost:    0,
			Hops:         0,
			VisitedCount: 1,
		}, nil
	}

	dist := map[NodeID]float64{opt.From: 0}
	prevEdge := make(map[NodeID]*Edge)
	settled := make(map[NodeID]struct{})

	h := newMinHeap(64)
	h.Push(opt.From, 0)

	res := &PathResult{}

	for h.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cur, curDist, ok := h.Pop()
		if !ok {
			break
		}
		if _, done := settled[cur]; done {
			continue
		}
		settled[cur] = struct{}{}
		res.VisitedCount++

		if cur == opt.To {
			break
		}
		if res.VisitedCount >= maxVisit {
			res.Truncated = true
			break
		}

		for _, adj := range g.adjacentLocked(cur, opt.Direction, f) {
			if _, done := settled[adj.To]; done {
				continue
			}
			nd := curDist + adj.Edge.Weight
			if old, seen := dist[adj.To]; !seen || nd < old {
				dist[adj.To] = nd
				prevEdge[adj.To] = adj.Edge
				h.Push(adj.To, nd)
			}
		}
	}

	total, reached := dist[opt.To]
	if !reached || math.IsInf(total, 1) {
		if _, settledTo := settled[opt.To]; !settledTo && res.Truncated {
			return res, nil
		}
		return res, nil
	}

	nodes, edges, err := g.rebuildPathLocked(opt.From, opt.To, prevEdge)
	if err != nil {
		return res, err
	}

	res.Found = true
	res.Nodes = nodes
	res.Edges = edges
	res.TotalCost = total
	res.Hops = len(edges)
	return res, nil
}

// rebuildPathLocked 从前驱边回溯出完整路径。调用方须持有读锁。
func (g *Graph) rebuildPathLocked(from, to NodeID, prevEdge map[NodeID]*Edge) ([]*Node, []*Edge, error) {
	revEdges := make([]*Edge, 0, 16)
	revNodes := []NodeID{to}

	cur := to
	// 步数上限等于节点总数，简单路径不可能超过它；一旦越界说明前驱链有环，属于内部错误。
	for i := 0; cur != from; i++ {
		if i > len(g.nodes) {
			return nil, nil, fmt.Errorf("路径回溯异常：前驱链超过节点总数，可能存在数据不一致")
		}
		e, ok := prevEdge[cur]
		if !ok {
			return nil, nil, fmt.Errorf("路径回溯异常：节点 %s 缺少前驱边", cur)
		}
		revEdges = append(revEdges, e)
		cur = otherEnd(e, cur)
		revNodes = append(revNodes, cur)
	}

	nodes := make([]*Node, 0, len(revNodes))
	for i := len(revNodes) - 1; i >= 0; i-- {
		n, ok := g.nodes[revNodes[i]]
		if !ok {
			return nil, nil, fmt.Errorf("路径回溯异常：节点 %s 不存在", revNodes[i])
		}
		nodes = append(nodes, n.Clone())
	}
	edges := make([]*Edge, 0, len(revEdges))
	for i := len(revEdges) - 1; i >= 0; i-- {
		edges = append(edges, revEdges[i].Clone())
	}
	return nodes, edges, nil
}

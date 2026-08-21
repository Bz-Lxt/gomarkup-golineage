package graph

import (
	"context"
	"fmt"
	"sort"
)

// TraverseOptions 遍历参数。
type TraverseOptions struct {
	// Start 起始节点 ID，必填。
	Start NodeID
	// MaxDepth 最大扩展深度。<=0 时取图的全局上限。
	MaxDepth int
	// Direction 遍历方向。
	Direction Direction
	// Relations 仅沿这些关系类型前进，空表示不过滤。
	Relations []RelationType
	// NodeTypes 仅访问这些类型的节点，空表示不过滤。
	NodeTypes []NodeType
	// MaxNodes 结果节点数上限。<=0 时取图的全局上限。
	MaxNodes int
}

func (o TraverseOptions) normalize(l Limits) TraverseOptions {
	if o.MaxDepth <= 0 || o.MaxDepth > l.MaxDepth {
		o.MaxDepth = l.MaxDepth
	}
	if o.MaxNodes <= 0 || o.MaxNodes > l.MaxNodes {
		o.MaxNodes = l.MaxNodes
	}
	return o
}

// TraverseResult 遍历结果。
type TraverseResult struct {
	// Algorithm 实际使用的算法标识（bfs / dfs）。
	Algorithm string `json:"algorithm"`
	// Nodes 访问到的节点（诱导子图的点集）。
	Nodes []*Node `json:"nodes"`
	// Edges 点集内部的全部边（诱导子图的边集），供前端完整还原局部拓扑。
	Edges []*Edge `json:"edges"`
	// Depths 各节点相对起点的深度。
	Depths map[NodeID]int `json:"depths"`
	// Order 节点的访问顺序。
	Order []NodeID `json:"order"`
	// Truncated 是否因触达上限而提前终止。
	Truncated bool `json:"truncated"`
	// CycleDetected 是否在遍历中发现环路（仅 DFS 有效）。
	CycleDetected bool `json:"cycle_detected"`
	// VisitedCount 实际访问的节点数。
	VisitedCount int `json:"visited_count"`
}

// BFS 广度优先遍历。
//
// 逐层扩展，Depths 中记录的是从起点出发的最少跳数。
func (g *Graph) BFS(ctx context.Context, opt TraverseOptions) (*TraverseResult, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[opt.Start]; !ok {
		return nil, fmt.Errorf("%w: 起点 id=%s", ErrNodeNotFound, opt.Start)
	}
	opt = opt.normalize(g.limits)
	f := newFilter(opt.Relations, opt.NodeTypes)

	res := &TraverseResult{
		Algorithm: "bfs",
		Depths:    map[NodeID]int{opt.Start: 0},
		Order:     make([]NodeID, 0, 64),
	}

	queue := []NodeID{opt.Start}
	visited := map[NodeID]struct{}{opt.Start: {}}

	for head := 0; head < len(queue); head++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cur := queue[head]
		res.Order = append(res.Order, cur)
		depth := res.Depths[cur]
		if depth >= opt.MaxDepth {
			continue
		}
		for _, adj := range g.adjacentLocked(cur, opt.Direction, f) {
			if _, seen := visited[adj.To]; seen {
				continue
			}
			if len(visited) >= opt.MaxNodes {
				res.Truncated = true
				break
			}
			visited[adj.To] = struct{}{}
			res.Depths[adj.To] = depth + 1
			queue = append(queue, adj.To)
		}
		if res.Truncated {
			break
		}
	}

	// 队列可能因截断而未被完全消费，补齐剩余节点的访问顺序。
	for i := len(res.Order); i < len(queue); i++ {
		res.Order = append(res.Order, queue[i])
	}

	res.VisitedCount = len(visited)
	res.Nodes = g.collectNodesLocked(visited)
	res.Edges = g.inducedEdgesLocked(visited, f)
	return res, nil
}

// dfsFrame 迭代式 DFS 的栈帧。
//
// 之所以不用递归：企业资产图存在长链路依赖，递归深度不可控，
// 迭代实现可以把深度约束显式化，也不会有栈溢出风险。
type dfsFrame struct {
	node    NodeID
	adjs    []adjEntry
	cursor  int
	depth   int
	visited bool
}

// 节点在 DFS 中的三态着色。gray 表示仍在当前搜索路径上，
// 指向 gray 节点的边即为回边，也就是环的判定依据。
const (
	colorWhite = iota
	colorGray
	colorBlack
)

// DFS 深度优先遍历，带环路检测。
func (g *Graph) DFS(ctx context.Context, opt TraverseOptions) (*TraverseResult, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[opt.Start]; !ok {
		return nil, fmt.Errorf("%w: 起点 id=%s", ErrNodeNotFound, opt.Start)
	}
	opt = opt.normalize(g.limits)
	f := newFilter(opt.Relations, opt.NodeTypes)

	res := &TraverseResult{
		Algorithm: "dfs",
		Depths:    map[NodeID]int{opt.Start: 0},
		Order:     make([]NodeID, 0, 64),
	}

	color := map[NodeID]int{opt.Start: colorGray}
	visited := map[NodeID]struct{}{opt.Start: {}}
	res.Order = append(res.Order, opt.Start)

	stack := []*dfsFrame{{
		node: opt.Start,
		adjs: g.adjacentLocked(opt.Start, opt.Direction, f),
	}}

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		top := stack[len(stack)-1]

		if top.cursor >= len(top.adjs) || top.depth >= opt.MaxDepth {
			color[top.node] = colorBlack
			stack = stack[:len(stack)-1]
			continue
		}

		adj := top.adjs[top.cursor]
		top.cursor++

		switch color[adj.To] {
		case colorGray:
			// 回边：目标仍在当前搜索路径上，说明存在环。
			res.CycleDetected = true
			continue
		case colorBlack:
			continue
		}

		if len(visited) >= opt.MaxNodes {
			res.Truncated = true
			break
		}

		color[adj.To] = colorGray
		visited[adj.To] = struct{}{}
		res.Depths[adj.To] = top.depth + 1
		res.Order = append(res.Order, adj.To)
		stack = append(stack, &dfsFrame{
			node:  adj.To,
			adjs:  g.adjacentLocked(adj.To, opt.Direction, f),
			depth: top.depth + 1,
		})
	}

	res.VisitedCount = len(visited)
	res.Nodes = g.collectNodesLocked(visited)
	res.Edges = g.inducedEdgesLocked(visited, f)
	return res, nil
}

// NeighborSubgraph 返回节点的 N 跳邻居子图，是前端「点击节点高亮相邻」的数据源。
//
// hops 为 1 时即一跳直接邻居。方向固定为双向：可视化关注的是「相关性」，
// 只看出边会漏掉上游依赖方。
func (g *Graph) NeighborSubgraph(ctx context.Context, id NodeID, hops int, relations []RelationType) (*TraverseResult, error) {
	if hops <= 0 {
		hops = 1
	}
	return g.BFS(ctx, TraverseOptions{
		Start:     id,
		MaxDepth:  hops,
		Direction: DirectionBoth,
		Relations: relations,
	})
}

// collectNodesLocked 按 ID 升序收集节点副本。调用方须持有读锁。
func (g *Graph) collectNodesLocked(ids map[NodeID]struct{}) []*Node {
	out := make([]*Node, 0, len(ids))
	for id := range ids {
		if n, ok := g.nodes[id]; ok {
			out = append(out, n.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// inducedEdgesLocked 提取诱导子图的边集：两端都落在点集内的全部边。
//
// 只返回遍历树边会让前端画布丢失横叉连接，看起来像是关系凭空消失，
// 因此这里返回完整的诱导边集。调用方须持有读锁。
func (g *Graph) inducedEdgesLocked(ids map[NodeID]struct{}, f *filter) []*Edge {
	seen := make(map[EdgeID]struct{})
	out := make([]*Edge, 0, len(ids))
	for id := range ids {
		a := g.adj[id]
		if a == nil {
			continue
		}
		for _, eid := range a.out {
			if _, dup := seen[eid]; dup {
				continue
			}
			e, ok := g.edges[eid]
			if !ok {
				continue
			}
			if !f.allowRelation(e.Relation) {
				continue
			}
			if _, ok := ids[e.Source]; !ok {
				continue
			}
			if _, ok := ids[e.Target]; !ok {
				continue
			}
			seen[eid] = struct{}{}
			out = append(out, e.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

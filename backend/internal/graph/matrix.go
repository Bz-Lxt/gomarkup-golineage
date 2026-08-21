package graph

import (
	"fmt"
	"sort"
)

// MaxMatrixNodes 邻接矩阵分析的节点数上限。
//
// 矩阵占用 O(V²)：500 节点约 25 万个单元尚可接受，
// 而全图 10 万节点会需要 10¹⁰ 个单元，这正是主存储必须用邻接表的原因。
const MaxMatrixNodes = 500

// MatrixAnalysis 子图的邻接矩阵分析结果。
type MatrixAnalysis struct {
	// NodeIDs 矩阵的行列顺序，第 i 行/列对应 NodeIDs[i]。
	NodeIDs []NodeID `json:"node_ids"`
	// NodeNames 与 NodeIDs 一一对应的名称，便于前端直接渲染热力图坐标轴。
	NodeNames []string `json:"node_names"`
	// Matrix 权重邻接矩阵，0 表示无连接。
	Matrix [][]float64 `json:"matrix"`
	// TwoHopReach 矩阵自乘的结果，TwoHopReach[i][j] 为 i 到 j 恰好两跳的路径条数。
	// 这是邻接矩阵相对邻接表的独有优势：多跳可达性可由矩阵幂直接得到。
	TwoHopReach [][]int `json:"two_hop_reach"`
	// Density 有向图密度 = 边数 / (V*(V-1))。
	Density float64 `json:"density"`
	// Components 弱连通分量，每个分量内的节点 ID 已排序。
	Components [][]NodeID `json:"components"`
	// ComponentCount 连通分量数量。
	ComponentCount int `json:"component_count"`
	// LargestComponent 最大连通分量的节点数。
	LargestComponent int `json:"largest_component"`
	// EdgeCount 子图内部的边数。
	EdgeCount int `json:"edge_count"`
}

// AnalyzeSubgraphMatrix 对给定节点集合构建邻接矩阵并做结构分析。
//
// 传入空集合时对全图分析，但全图节点数超过 MaxMatrixNodes 会直接拒绝，
// 而不是悄悄截断 —— 截断后的密度与连通分量都是错误结论。
func (g *Graph) AnalyzeSubgraphMatrix(ids []NodeID) (*MatrixAnalysis, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var selected []NodeID
	if len(ids) == 0 {
		selected = make([]NodeID, 0, len(g.nodes))
		for id := range g.nodes {
			selected = append(selected, id)
		}
	} else {
		seen := make(map[NodeID]struct{}, len(ids))
		for _, id := range ids {
			if _, dup := seen[id]; dup {
				continue
			}
			if _, ok := g.nodes[id]; !ok {
				return nil, fmt.Errorf("%w: id=%s", ErrNodeNotFound, id)
			}
			seen[id] = struct{}{}
			selected = append(selected, id)
		}
	}

	if len(selected) == 0 {
		return &MatrixAnalysis{
			NodeIDs: []NodeID{}, NodeNames: []string{},
			Matrix: [][]float64{}, TwoHopReach: [][]int{}, Components: [][]NodeID{},
		}, nil
	}
	if len(selected) > MaxMatrixNodes {
		return nil, fmt.Errorf("%w: 当前 %d 个节点，上限 %d（请先用邻居子图缩小范围）",
			ErrSubgraphTooLarge, len(selected), MaxMatrixNodes)
	}

	sort.Strings(selected)
	n := len(selected)
	pos := make(map[NodeID]int, n)
	names := make([]string, n)
	for i, id := range selected {
		pos[id] = i
		names[i] = g.nodes[id].Name
	}

	matrix := make([][]float64, n)
	for i := range matrix {
		matrix[i] = make([]float64, n)
	}

	inSet := make(map[NodeID]struct{}, n)
	for _, id := range selected {
		inSet[id] = struct{}{}
	}

	edgeCount := 0
	seenEdge := make(map[EdgeID]struct{})
	for _, id := range selected {
		a := g.adj[id]
		if a == nil {
			continue
		}
		for _, eid := range a.out {
			if _, dup := seenEdge[eid]; dup {
				continue
			}
			e, ok := g.edges[eid]
			if !ok {
				continue
			}
			si, sok := pos[e.Source]
			ti, tok := pos[e.Target]
			if !sok || !tok {
				continue
			}
			seenEdge[eid] = struct{}{}
			edgeCount++
			matrix[si][ti] = e.Weight
			if !e.Directed {
				matrix[ti][si] = e.Weight
			}
		}
	}

	res := &MatrixAnalysis{
		NodeIDs:   selected,
		NodeNames: names,
		Matrix:    matrix,
		EdgeCount: edgeCount,
	}
	if n > 1 {
		res.Density = float64(edgeCount) / float64(n*(n-1))
	}
	res.TwoHopReach = squareBoolean(matrix)
	res.Components = weaklyConnectedComponents(matrix, selected)
	res.ComponentCount = len(res.Components)
	for _, c := range res.Components {
		if len(c) > res.LargestComponent {
			res.LargestComponent = len(c)
		}
	}
	return res, nil
}

// squareBoolean 计算邻接矩阵的布尔平方：结果 [i][j] 为 i 到 j 恰好两跳的路径条数。
func squareBoolean(m [][]float64) [][]int {
	n := len(m)
	out := make([][]int, n)
	for i := range out {
		out[i] = make([]int, n)
	}
	for i := 0; i < n; i++ {
		for k := 0; k < n; k++ {
			if m[i][k] == 0 {
				continue
			}
			for j := 0; j < n; j++ {
				if m[k][j] != 0 {
					out[i][j]++
				}
			}
		}
	}
	return out
}

// weaklyConnectedComponents 忽略边方向求连通分量，使用并查集。
func weaklyConnectedComponents(m [][]float64, ids []NodeID) [][]NodeID {
	n := len(ids)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]] // 路径压缩
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if m[i][j] != 0 {
				union(i, j)
			}
		}
	}

	groups := make(map[int][]NodeID)
	for i := 0; i < n; i++ {
		r := find(i)
		groups[r] = append(groups[r], ids[i])
	}

	out := make([][]NodeID, 0, len(groups))
	for _, g := range groups {
		component := make([]NodeID, len(g))
		copy(component, g)
		sort.Strings(component)
		out = append(out, component)
	}
	// 大分量优先，便于前端展示主结构。
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i][0] < out[j][0]
	})
	return out
}

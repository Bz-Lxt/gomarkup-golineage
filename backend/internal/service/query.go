package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/alkaid/golineage/internal/graph"
)

// withQueryTimeout 为图查询套上超时上下文。
//
// 超时与客户端取消都必须能及时终止计算：稠密图上的 Dijkstra / 全路径枚举
// 是 CPU 密集型循环，若已取消的查询继续跑到自身超时，会长期占用 CPU。
// 因此派生上下文仍派生自请求上下文 —— 客户端断连或前端取消上一条请求时，
// 取消信号会立即传播到图算法的 ctx.Err() 检查点，而不必等到超时兜底。
func (s *Service) withQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.cfg.GraphQueryTimout)
}

// TopologyQuery 全量拓扑查询参数。
type TopologyQuery struct {
	Types     []string
	Relations []string
	Limit     int
}

// Topology 返回（可能被裁剪的）全量拓扑。
func (s *Service) Topology(ctx context.Context, q TopologyQuery) (*graph.Topology, error) {
	types, err := parseNodeTypes(q.Types)
	if err != nil {
		return nil, err
	}
	rels, err := parseRelationTypes(q.Relations)
	if err != nil {
		return nil, err
	}
	return s.repo.Topology(ctx, graph.TopologyOptions{
		Types: types, Relations: rels, Limit: q.Limit,
	}), nil
}

// SearchQuery 节点检索参数。
type SearchQuery struct {
	Keyword    string
	NamePrefix string
	Types      []string
	PropKey    string
	PropValue  string
	Limit      int
}

// SearchNodes 按条件检索资产节点。
func (s *Service) SearchNodes(ctx context.Context, q SearchQuery) ([]*graph.Node, error) {
	types, err := parseNodeTypes(q.Types)
	if err != nil {
		return nil, err
	}
	opt := graph.SearchOptions{
		Keyword:    q.Keyword,
		NamePrefix: q.NamePrefix,
		Types:      types,
		PropKey:    strings.TrimSpace(q.PropKey),
		Limit:      q.Limit,
	}
	if opt.PropKey != "" && q.PropValue != "" {
		opt.PropValue = q.PropValue
	}
	return s.repo.Search(ctx, opt), nil
}

// TraverseQuery 遍历查询参数。
type TraverseQuery struct {
	// Algorithm 取值 bfs 或 dfs，空值默认 bfs。
	Algorithm string
	Start     string
	MaxDepth  int
	Direction string
	Relations []string
	NodeTypes []string
	MaxNodes  int
}

// Traverse 执行 BFS 或 DFS 遍历。
func (s *Service) Traverse(ctx context.Context, q TraverseQuery) (*graph.TraverseResult, error) {
	dir, err := graph.ParseDirection(q.Direction)
	if err != nil {
		return nil, err
	}
	rels, err := parseRelationTypes(q.Relations)
	if err != nil {
		return nil, err
	}
	types, err := parseNodeTypes(q.NodeTypes)
	if err != nil {
		return nil, err
	}

	opt := graph.TraverseOptions{
		Start:     strings.TrimSpace(q.Start),
		MaxDepth:  q.MaxDepth,
		Direction: dir,
		Relations: rels,
		NodeTypes: types,
		MaxNodes:  q.MaxNodes,
	}
	if opt.Start == "" {
		return nil, fmt.Errorf("%w: 起点节点 ID 不能为空", graph.ErrValidation)
	}

	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(q.Algorithm)) {
	case "", "bfs":
		return s.repo.BFS(ctx, opt)
	case "dfs":
		return s.repo.DFS(ctx, opt)
	default:
		return nil, fmt.Errorf("%w: 遍历算法 %q 非法（可选 bfs/dfs）", graph.ErrValidation, q.Algorithm)
	}
}

// Neighbors 返回节点的 N 跳邻居子图，是前端点击高亮的数据源。
func (s *Service) Neighbors(ctx context.Context, id string, hops int, relations []string) (*graph.TraverseResult, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: 节点 ID 不能为空", graph.ErrValidation)
	}
	rels, err := parseRelationTypes(relations)
	if err != nil {
		return nil, err
	}
	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()
	return s.repo.Neighbors(ctx, id, hops, rels)
}

// PathQuery 路径查询参数。
type PathQuery struct {
	From      string
	To        string
	Direction string
	Relations []string
	MaxDepth  int
	MaxPaths  int
}

func (q PathQuery) validate() error {
	if strings.TrimSpace(q.From) == "" || strings.TrimSpace(q.To) == "" {
		return fmt.Errorf("%w: 起点与终点均不能为空", graph.ErrValidation)
	}
	return nil
}

// ShortestPath 计算两点间代价最小的路径。
func (s *Service) ShortestPath(ctx context.Context, q PathQuery) (*graph.PathResult, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	dir, err := graph.ParseDirection(q.Direction)
	if err != nil {
		return nil, err
	}
	rels, err := parseRelationTypes(q.Relations)
	if err != nil {
		return nil, err
	}

	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()
	return s.repo.ShortestPath(ctx, graph.PathOptions{
		From: strings.TrimSpace(q.From), To: strings.TrimSpace(q.To),
		Direction: dir, Relations: rels,
	})
}

// AllPaths 枚举两点间的全部简单路径。
func (s *Service) AllPaths(ctx context.Context, q PathQuery) (*graph.AllPathsResult, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	dir, err := graph.ParseDirection(q.Direction)
	if err != nil {
		return nil, err
	}
	rels, err := parseRelationTypes(q.Relations)
	if err != nil {
		return nil, err
	}

	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()
	return s.repo.AllPaths(ctx, graph.AllPathsOptions{
		From: strings.TrimSpace(q.From), To: strings.TrimSpace(q.To),
		Direction: dir, Relations: rels,
		MaxDepth: q.MaxDepth, MaxPaths: q.MaxPaths,
	})
}

// LineageQuery 血缘分析参数。
type LineageQuery struct {
	Root      string
	MaxDepth  int
	Relations []string
	MaxNodes  int
}

// Lineage 计算资产的上下游血缘。
func (s *Service) Lineage(ctx context.Context, q LineageQuery) (*graph.LineageResult, error) {
	if strings.TrimSpace(q.Root) == "" {
		return nil, fmt.Errorf("%w: 分析起点不能为空", graph.ErrValidation)
	}
	rels, err := parseRelationTypes(q.Relations)
	if err != nil {
		return nil, err
	}

	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()
	return s.repo.Lineage(ctx, graph.LineageOptions{
		Root: strings.TrimSpace(q.Root), MaxDepth: q.MaxDepth,
		Relations: rels, MaxNodes: q.MaxNodes,
	})
}

// Impact 汇总节点的影响规模，用于删除前的风险提示。
func (s *Service) Impact(ctx context.Context, id string, maxDepth int) (*graph.ImpactSummary, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: 节点 ID 不能为空", graph.ErrValidation)
	}
	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()
	return s.repo.Impact(ctx, id, maxDepth)
}

// MatrixAnalysis 对子图做邻接矩阵结构分析。
func (s *Service) MatrixAnalysis(ctx context.Context, ids []string) (*graph.MatrixAnalysis, error) {
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			cleaned = append(cleaned, id)
		}
	}
	ctx, cancel := s.withQueryTimeout(ctx)
	defer cancel()
	return s.repo.MatrixAnalysis(ctx, cleaned)
}

// StructureOverview 图的结构概览。
type StructureOverview struct {
	Stats          graph.Stats   `json:"stats"`
	Roots          []*graph.Node `json:"roots"`
	Leaves         []*graph.Node `json:"leaves"`
	HasCycle       bool          `json:"has_cycle"`
	CycleHint      string        `json:"cycle_hint,omitempty"`
	TopologicalLen int           `json:"topological_length"`
}

// Structure 汇总图的结构特征：源头资产、末端资产与循环依赖检测。
func (s *Service) Structure(ctx context.Context) *StructureOverview {
	roots, leaves := s.repo.RootsAndLeaves(ctx)
	ov := &StructureOverview{
		Stats:  s.repo.Stats(ctx),
		Roots:  limitNodes(roots, 50),
		Leaves: limitNodes(leaves, 50),
	}
	order, err := s.repo.TopologicalOrder(ctx)
	ov.TopologicalLen = len(order)
	if err != nil {
		ov.HasCycle = true
		ov.CycleHint = err.Error()
	}
	return ov
}

func limitNodes(ns []*graph.Node, n int) []*graph.Node {
	if ns == nil {
		return []*graph.Node{}
	}
	if len(ns) > n {
		return ns[:n]
	}
	return ns
}

// GetNode 读取单个资产节点。
func (s *Service) GetNode(ctx context.Context, id string) (*graph.Node, error) {
	return s.repo.GetNode(ctx, id)
}

// GetEdge 读取单条血缘关系。
func (s *Service) GetEdge(ctx context.Context, id string) (*graph.Edge, error) {
	return s.repo.GetEdge(ctx, id)
}

// NodeDetail 节点详情，附带度数与影响面统计。
type NodeDetail struct {
	Node       *graph.Node          `json:"node"`
	InDegree   int                  `json:"in_degree"`
	OutDegree  int                  `json:"out_degree"`
	Impact     *graph.ImpactSummary `json:"impact"`
	Incident   []*graph.Edge        `json:"incident_edges"`
	EventCount int64                `json:"event_count"`
}

// NodeDetail 汇总资产详情，供右侧抽屉一次性渲染。
func (s *Service) NodeDetail(ctx context.Context, id string) (*NodeDetail, error) {
	n, err := s.repo.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}

	d := &NodeDetail{Node: n}
	if in, err := s.repo.Degree(ctx, id, graph.DirectionIn); err == nil {
		d.InDegree = in
	}
	if out, err := s.repo.Degree(ctx, id, graph.DirectionOut); err == nil {
		d.OutDegree = out
	}
	if inc, err := s.repo.IncidentEdges(ctx, id); err == nil {
		d.Incident = inc
	}
	if imp, err := s.Impact(ctx, id, 0); err == nil {
		d.Impact = imp
	}
	if _, total, err := s.store.List(ctx, listFilterForEntity(id)); err == nil {
		d.EventCount = total
	}
	return d, nil
}

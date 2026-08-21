// Package repository 定义图存储的抽象端口（Port）及其内存实现。
//
// 这一层存在的意义是「图数据库适配」：Service 只依赖 GraphRepository 接口，
// 未来若要接入 Neo4j / NebulaGraph，只需新增一个 Adapter 实现本接口，
// 上层业务代码无需改动。当前唯一实现是包裹手写内存图引擎的 MemoryGraphAdapter。
package repository

import (
	"context"

	"github.com/alkaid/golineage/internal/graph"
)

// GraphRepository 图存储端口。
//
// 所有方法都接收 context：图算法可能长时间运行，调用方需要能够超时取消。
type GraphRepository interface {
	// Name 返回适配器标识，用于健康检查与诊断信息。
	Name() string

	// ---- 实体读取 ----
	GetNode(ctx context.Context, id string) (*graph.Node, error)
	GetEdge(ctx context.Context, id string) (*graph.Edge, error)
	HasNode(ctx context.Context, id string) bool
	// IncidentEdges 返回与节点相连的全部边，用于删除前预知级联影响。
	IncidentEdges(ctx context.Context, id string) ([]*graph.Edge, error)
	Search(ctx context.Context, opt graph.SearchOptions) []*graph.Node
	Topology(ctx context.Context, opt graph.TopologyOptions) *graph.Topology
	Stats(ctx context.Context) graph.Stats
	PropertyKeys(ctx context.Context) []string
	Degree(ctx context.Context, id string, dir graph.Direction) (int, error)

	// ---- 实体写入 ----
	// 这些方法只改内存投影，不负责写事件日志；
	// 事件的生成与持久化由 Service 层统一编排，避免职责下沉导致漏记流水。
	AddNode(ctx context.Context, n *graph.Node) error
	UpdateNode(ctx context.Context, n *graph.Node) error
	RemoveNode(ctx context.Context, id string) ([]*graph.Edge, error)
	AddEdge(ctx context.Context, e *graph.Edge) error
	UpdateEdge(ctx context.Context, e *graph.Edge) error
	RemoveEdge(ctx context.Context, id string) (*graph.Edge, error)

	// ---- 图算法 ----
	BFS(ctx context.Context, opt graph.TraverseOptions) (*graph.TraverseResult, error)
	DFS(ctx context.Context, opt graph.TraverseOptions) (*graph.TraverseResult, error)
	Neighbors(ctx context.Context, id string, hops int, rels []graph.RelationType) (*graph.TraverseResult, error)
	ShortestPath(ctx context.Context, opt graph.PathOptions) (*graph.PathResult, error)
	AllPaths(ctx context.Context, opt graph.AllPathsOptions) (*graph.AllPathsResult, error)
	Lineage(ctx context.Context, opt graph.LineageOptions) (*graph.LineageResult, error)
	Impact(ctx context.Context, id string, maxDepth int) (*graph.ImpactSummary, error)
	MatrixAnalysis(ctx context.Context, ids []string) (*graph.MatrixAnalysis, error)
	TopologicalOrder(ctx context.Context) ([]*graph.Node, error)
	RootsAndLeaves(ctx context.Context) (roots, leaves []*graph.Node)

	// ---- 快照 ----
	Snapshot(ctx context.Context) *graph.Snapshot
	Limits() graph.Limits

	// Underlying 暴露底层图实例，供事件重放直接写入。
	//
	// 这是一处刻意的抽象泄漏：重放需要绕过业务校验按事件序列强制重建状态，
	// 走常规写入方法会因「节点已存在」之类的中间态差异而失败。
	Underlying() *graph.Graph
}

// MemoryGraphAdapter 基于手写内存图引擎的 GraphRepository 实现。
type MemoryGraphAdapter struct {
	g *graph.Graph
}

var _ GraphRepository = (*MemoryGraphAdapter)(nil)

// NewMemoryGraphAdapter 创建内存图适配器。
func NewMemoryGraphAdapter(g *graph.Graph) *MemoryGraphAdapter {
	return &MemoryGraphAdapter{g: g}
}

// Name 返回适配器标识。
func (a *MemoryGraphAdapter) Name() string { return "memory-adjacency-list" }

// Underlying 返回底层图实例。
func (a *MemoryGraphAdapter) Underlying() *graph.Graph { return a.g }

// Limits 返回图的安全上限配置。
func (a *MemoryGraphAdapter) Limits() graph.Limits { return a.g.Limits() }

func (a *MemoryGraphAdapter) GetNode(_ context.Context, id string) (*graph.Node, error) {
	return a.g.GetNode(id)
}

func (a *MemoryGraphAdapter) GetEdge(_ context.Context, id string) (*graph.Edge, error) {
	return a.g.GetEdge(id)
}

func (a *MemoryGraphAdapter) HasNode(_ context.Context, id string) bool { return a.g.HasNode(id) }

func (a *MemoryGraphAdapter) IncidentEdges(_ context.Context, id string) ([]*graph.Edge, error) {
	return a.g.IncidentEdges(id)
}

func (a *MemoryGraphAdapter) Search(_ context.Context, opt graph.SearchOptions) []*graph.Node {
	return a.g.Search(opt)
}

func (a *MemoryGraphAdapter) Topology(_ context.Context, opt graph.TopologyOptions) *graph.Topology {
	return a.g.Topology(opt)
}

func (a *MemoryGraphAdapter) Stats(context.Context) graph.Stats { return a.g.Stats() }

func (a *MemoryGraphAdapter) PropertyKeys(context.Context) []string { return a.g.PropertyKeys() }

func (a *MemoryGraphAdapter) Degree(_ context.Context, id string, dir graph.Direction) (int, error) {
	return a.g.Degree(id, dir)
}

func (a *MemoryGraphAdapter) AddNode(_ context.Context, n *graph.Node) error { return a.g.AddNode(n) }

func (a *MemoryGraphAdapter) UpdateNode(_ context.Context, n *graph.Node) error {
	return a.g.UpdateNode(n)
}

func (a *MemoryGraphAdapter) RemoveNode(_ context.Context, id string) ([]*graph.Edge, error) {
	return a.g.RemoveNode(id)
}

func (a *MemoryGraphAdapter) AddEdge(_ context.Context, e *graph.Edge) error { return a.g.AddEdge(e) }

func (a *MemoryGraphAdapter) UpdateEdge(_ context.Context, e *graph.Edge) error {
	return a.g.UpdateEdge(e)
}

func (a *MemoryGraphAdapter) RemoveEdge(_ context.Context, id string) (*graph.Edge, error) {
	return a.g.RemoveEdge(id)
}

func (a *MemoryGraphAdapter) BFS(ctx context.Context, opt graph.TraverseOptions) (*graph.TraverseResult, error) {
	return a.g.BFS(ctx, opt)
}

func (a *MemoryGraphAdapter) DFS(ctx context.Context, opt graph.TraverseOptions) (*graph.TraverseResult, error) {
	return a.g.DFS(ctx, opt)
}

func (a *MemoryGraphAdapter) Neighbors(ctx context.Context, id string, hops int, rels []graph.RelationType) (*graph.TraverseResult, error) {
	return a.g.NeighborSubgraph(ctx, id, hops, rels)
}

func (a *MemoryGraphAdapter) ShortestPath(ctx context.Context, opt graph.PathOptions) (*graph.PathResult, error) {
	return a.g.ShortestPath(ctx, opt)
}

func (a *MemoryGraphAdapter) AllPaths(ctx context.Context, opt graph.AllPathsOptions) (*graph.AllPathsResult, error) {
	return a.g.AllPaths(ctx, opt)
}

func (a *MemoryGraphAdapter) Lineage(ctx context.Context, opt graph.LineageOptions) (*graph.LineageResult, error) {
	return a.g.Lineage(ctx, opt)
}

func (a *MemoryGraphAdapter) Impact(ctx context.Context, id string, maxDepth int) (*graph.ImpactSummary, error) {
	return a.g.Impact(ctx, id, maxDepth)
}

func (a *MemoryGraphAdapter) MatrixAnalysis(_ context.Context, ids []string) (*graph.MatrixAnalysis, error) {
	return a.g.AnalyzeSubgraphMatrix(ids)
}

func (a *MemoryGraphAdapter) TopologicalOrder(context.Context) ([]*graph.Node, error) {
	return a.g.TopologicalOrder()
}

func (a *MemoryGraphAdapter) RootsAndLeaves(context.Context) ([]*graph.Node, []*graph.Node) {
	return a.g.RootsAndLeaves()
}

func (a *MemoryGraphAdapter) Snapshot(context.Context) *graph.Snapshot { return a.g.Snapshot() }

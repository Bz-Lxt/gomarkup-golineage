package graph

import (
	"fmt"
	"sort"
	"sync"
)

// Limits 图算法的安全上限，防止一次超宽查询拖垮整个服务。
//
// Limits 会随 /api/v1/meta 下发给前端用于约束输入控件，
// 因此字段必须带 json tag 以保持与其余响应一致的 snake_case 风格。
type Limits struct {
	MaxDepth int `json:"max_depth"` // 最大遍历深度
	MaxPaths int `json:"max_paths"` // 全路径枚举的最大路径条数
	MaxNodes int `json:"max_nodes"` // 单次查询最多返回的节点数
}

// DefaultLimits 返回一组保守的默认上限。
func DefaultLimits() Limits {
	return Limits{MaxDepth: 10, MaxPaths: 1000, MaxNodes: 20000}
}

func (l Limits) normalize() Limits {
	if l.MaxDepth <= 0 {
		l.MaxDepth = 10
	}
	if l.MaxPaths <= 0 {
		l.MaxPaths = 1000
	}
	if l.MaxNodes <= 0 {
		l.MaxNodes = 20000
	}
	return l
}

// adjacency 单个节点的邻接信息。
//
// out / in 均为按 EdgeID 升序排列的切片。使用切片而非 map 有三个理由：
// 稀疏图下度数很小，二分查找与线性删除的开销可忽略；内存占用显著低于 map；
// 最关键的是迭代顺序确定，使遍历结果可复现。
type adjacency struct {
	out []EdgeID
	in  []EdgeID
}

// Graph 基于邻接表的并发安全有向带权图。
//
// 无向边（Directed == false）在两侧的出边表与入边表中对称登记，
// 使遍历逻辑无需为方向性做特殊分支；代价是删除时必须对称清理。
type Graph struct {
	mu     sync.RWMutex
	nodes  map[NodeID]*Node
	edges  map[EdgeID]*Edge
	adj    map[NodeID]*adjacency
	uniq   map[string]EdgeID // 边去重键 -> 边 ID
	idx    *index
	limits Limits
}

// New 创建空图。
func New(limits Limits) *Graph {
	return &Graph{
		nodes:  make(map[NodeID]*Node),
		edges:  make(map[EdgeID]*Edge),
		adj:    make(map[NodeID]*adjacency),
		uniq:   make(map[string]EdgeID),
		idx:    newIndex(),
		limits: limits.normalize(),
	}
}

// Limits 返回当前生效的安全上限。
func (g *Graph) Limits() Limits { return g.limits }

// Reset 清空全图。事件重放前会先调用它，保证重建结果不受残留状态影响。
func (g *Graph) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes = make(map[NodeID]*Node)
	g.edges = make(map[EdgeID]*Edge)
	g.adj = make(map[NodeID]*adjacency)
	g.uniq = make(map[string]EdgeID)
	g.idx.reset()
}

// ---------------------------------------------------------------- //
// 节点操作
// ---------------------------------------------------------------- //

// AddNode 新增节点。ID 已存在时返回 ErrNodeExists。
func (g *Graph) AddNode(n *Node) error {
	if err := n.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.nodes[n.ID]; exists {
		return fmt.Errorf("%w: id=%s", ErrNodeExists, n.ID)
	}
	g.putNodeLocked(n.Clone())
	return nil
}

// PutNode 幂等地写入节点：不存在则创建，存在则整体覆盖。
//
// 事件重放使用此方法而非 AddNode —— 重放要求结果只取决于事件序列本身，
// 不能因为「已存在」这类中间态差异而中断。
func (g *Graph) PutNode(n *Node) error {
	if err := n.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if old, exists := g.nodes[n.ID]; exists {
		g.idx.remove(old)
	}
	g.putNodeLocked(n.Clone())
	return nil
}

func (g *Graph) putNodeLocked(n *Node) {
	g.nodes[n.ID] = n
	if _, ok := g.adj[n.ID]; !ok {
		g.adj[n.ID] = &adjacency{}
	}
	g.idx.add(n)
}

// UpdateNode 更新节点。节点不存在时返回 ErrNodeNotFound。
func (g *Graph) UpdateNode(n *Node) error {
	if err := n.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	old, exists := g.nodes[n.ID]
	if !exists {
		return fmt.Errorf("%w: id=%s", ErrNodeNotFound, n.ID)
	}
	g.idx.remove(old)
	g.putNodeLocked(n.Clone())
	return nil
}

// RemoveNode 删除节点，并级联删除其全部关联边。
//
// 返回被级联删除的边（副本），上层据此为每条边生成独立的删除事件 ——
// 少了这些事件，历史回溯重建出的拓扑会残留悬空边。
func (g *Graph) RemoveNode(id NodeID) ([]*Edge, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	n, exists := g.nodes[id]
	if !exists {
		return nil, fmt.Errorf("%w: id=%s", ErrNodeNotFound, id)
	}

	victims := make([]EdgeID, 0, 8)
	if a, ok := g.adj[id]; ok {
		seen := make(map[EdgeID]struct{}, len(a.out)+len(a.in))
		for _, eid := range a.out {
			seen[eid] = struct{}{}
		}
		for _, eid := range a.in {
			seen[eid] = struct{}{}
		}
		for eid := range seen {
			victims = append(victims, eid)
		}
		sort.Strings(victims)
	}

	removed := make([]*Edge, 0, len(victims))
	for _, eid := range victims {
		if e, ok := g.edges[eid]; ok {
			removed = append(removed, e.Clone())
			g.removeEdgeLocked(e)
		}
	}

	g.idx.remove(n)
	delete(g.nodes, id)
	delete(g.adj, id)
	return removed, nil
}

// GetNode 返回节点副本。
func (g *Graph) GetNode(id NodeID) (*Node, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	if !ok {
		return nil, fmt.Errorf("%w: id=%s", ErrNodeNotFound, id)
	}
	return n.Clone(), nil
}

// HasNode 报告节点是否存在。
func (g *Graph) HasNode(id NodeID) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.nodes[id]
	return ok
}

// Nodes 返回全部节点副本，按 ID 升序。
func (g *Graph) Nodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodesLocked()
}

func (g *Graph) nodesLocked() []*Node {
	out := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ---------------------------------------------------------------- //
// 边操作
// ---------------------------------------------------------------- //

// uniqKey 构造边去重键。无向边额外做端点归一，避免 A-B 与 B-A 被视为两条边。
func uniqKey(e *Edge) string {
	s, t := e.Source, e.Target
	if !e.Directed && s > t {
		s, t = t, s
	}
	return s + "\x00" + t + "\x00" + string(e.Relation)
}

// AddEdge 新增边。端点必须已存在，且不允许重复边。
func (g *Graph) AddEdge(e *Edge) error {
	if err := e.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.edges[e.ID]; exists {
		return fmt.Errorf("%w: id=%s", ErrEdgeExists, e.ID)
	}
	if _, ok := g.nodes[e.Source]; !ok {
		return fmt.Errorf("%w: 起点 id=%s", ErrNodeNotFound, e.Source)
	}
	if _, ok := g.nodes[e.Target]; !ok {
		return fmt.Errorf("%w: 终点 id=%s", ErrNodeNotFound, e.Target)
	}
	if dup, ok := g.uniq[uniqKey(e)]; ok {
		return fmt.Errorf("%w: %s -[%s]-> %s 已存在（id=%s）", ErrEdgeExists, e.Source, e.Relation, e.Target, dup)
	}

	g.putEdgeLocked(e.Clone())
	return nil
}

// PutEdge 幂等地写入边，供事件重放使用。
func (g *Graph) PutEdge(e *Edge) error {
	if err := e.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.nodes[e.Source]; !ok {
		return fmt.Errorf("%w: 起点 id=%s", ErrNodeNotFound, e.Source)
	}
	if _, ok := g.nodes[e.Target]; !ok {
		return fmt.Errorf("%w: 终点 id=%s", ErrNodeNotFound, e.Target)
	}
	if old, exists := g.edges[e.ID]; exists {
		g.removeEdgeLocked(old)
	}
	g.putEdgeLocked(e.Clone())
	return nil
}

func (g *Graph) putEdgeLocked(e *Edge) {
	g.edges[e.ID] = e
	g.uniq[uniqKey(e)] = e.ID

	src := g.ensureAdjLocked(e.Source)
	dst := g.ensureAdjLocked(e.Target)

	src.out = insertSorted(src.out, e.ID)
	dst.in = insertSorted(dst.in, e.ID)

	// 无向边在反方向对称登记，使遍历时无需再判断方向性。
	if !e.Directed {
		dst.out = insertSorted(dst.out, e.ID)
		src.in = insertSorted(src.in, e.ID)
	}
}

func (g *Graph) removeEdgeLocked(e *Edge) {
	delete(g.edges, e.ID)
	if cur, ok := g.uniq[uniqKey(e)]; ok && cur == e.ID {
		delete(g.uniq, uniqKey(e))
	}
	if a, ok := g.adj[e.Source]; ok {
		a.out = removeSorted(a.out, e.ID)
		a.in = removeSorted(a.in, e.ID)
	}
	if a, ok := g.adj[e.Target]; ok {
		a.out = removeSorted(a.out, e.ID)
		a.in = removeSorted(a.in, e.ID)
	}
}

func (g *Graph) ensureAdjLocked(id NodeID) *adjacency {
	a, ok := g.adj[id]
	if !ok {
		a = &adjacency{}
		g.adj[id] = a
	}
	return a
}

// UpdateEdge 更新边。端点不可变更，只允许修改权重、关系类型与属性。
func (g *Graph) UpdateEdge(e *Edge) error {
	if err := e.Validate(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	old, exists := g.edges[e.ID]
	if !exists {
		return fmt.Errorf("%w: id=%s", ErrEdgeNotFound, e.ID)
	}
	if old.Source != e.Source || old.Target != e.Target {
		return fmt.Errorf("%w: 不允许修改边的端点，请删除后重建", ErrValidation)
	}
	// 关系类型变化可能与既有边撞键，需要提前拦截。
	if nk := uniqKey(e); nk != uniqKey(old) {
		if dup, ok := g.uniq[nk]; ok && dup != e.ID {
			return fmt.Errorf("%w: %s -[%s]-> %s 已存在（id=%s）", ErrEdgeExists, e.Source, e.Relation, e.Target, dup)
		}
	}

	g.removeEdgeLocked(old)
	g.putEdgeLocked(e.Clone())
	return nil
}

// RemoveEdge 删除边。
func (g *Graph) RemoveEdge(id EdgeID) (*Edge, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.edges[id]
	if !ok {
		return nil, fmt.Errorf("%w: id=%s", ErrEdgeNotFound, id)
	}
	clone := e.Clone()
	g.removeEdgeLocked(e)
	return clone, nil
}

// GetEdge 返回边副本。
func (g *Graph) GetEdge(id EdgeID) (*Edge, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	e, ok := g.edges[id]
	if !ok {
		return nil, fmt.Errorf("%w: id=%s", ErrEdgeNotFound, id)
	}
	return e.Clone(), nil
}

// IncidentEdges 返回与节点相连的全部边（出边与入边，已去重，按 ID 升序）。
//
// 删除节点前需要预知哪些关系会被级联移除，以便为每条边生成独立的删除事件。
// 这必须是只读操作：事件溯源要求日志先于内存变更落盘，
// 若靠「先删再看返回值」来获取列表，就颠倒了这个顺序。
func (g *Graph) IncidentEdges(id NodeID) ([]*Edge, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[id]; !ok {
		return nil, fmt.Errorf("%w: id=%s", ErrNodeNotFound, id)
	}
	a := g.adj[id]
	if a == nil {
		return []*Edge{}, nil
	}

	seen := make(map[EdgeID]struct{}, len(a.out)+len(a.in))
	out := make([]*Edge, 0, len(a.out)+len(a.in))
	for _, ids := range [][]EdgeID{a.out, a.in} {
		for _, eid := range ids {
			if _, dup := seen[eid]; dup {
				continue
			}
			seen[eid] = struct{}{}
			if e, ok := g.edges[eid]; ok {
				out = append(out, e.Clone())
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Edges 返回全部边副本，按 ID 升序。
func (g *Graph) Edges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.edgesLocked()
}

func (g *Graph) edgesLocked() []*Edge {
	out := make([]*Edge, 0, len(g.edges))
	for _, e := range g.edges {
		out = append(out, e.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ---------------------------------------------------------------- //
// 统计与邻接查询
// ---------------------------------------------------------------- //

// NodeCount 返回节点总数。
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// EdgeCount 返回边总数。
func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.edges)
}

// Degree 返回节点在指定方向上的度数。
func (g *Graph) Degree(id NodeID, dir Direction) (int, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if _, ok := g.nodes[id]; !ok {
		return 0, fmt.Errorf("%w: id=%s", ErrNodeNotFound, id)
	}
	a := g.adj[id]
	if a == nil {
		return 0, nil
	}
	switch dir {
	case DirectionIn:
		return len(a.in), nil
	case DirectionBoth:
		seen := make(map[EdgeID]struct{}, len(a.out)+len(a.in))
		for _, e := range a.out {
			seen[e] = struct{}{}
		}
		for _, e := range a.in {
			seen[e] = struct{}{}
		}
		return len(seen), nil
	default:
		return len(a.out), nil
	}
}

// Stats 图规模概览。
type Stats struct {
	NodeCount  int              `json:"node_count"`
	EdgeCount  int              `json:"edge_count"`
	TypeCounts map[NodeType]int `json:"type_counts"`
	AvgDegree  float64          `json:"avg_degree"`
	MaxDegree  int              `json:"max_degree"`
	Isolated   int              `json:"isolated_count"`
}

// Stats 计算图的规模统计。
func (g *Graph) Stats() Stats {
	g.mu.RLock()
	defer g.mu.RUnlock()

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
		if a == nil {
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
		d := len(seen)
		if d == 0 {
			s.Isolated++
		}
		if d > s.MaxDegree {
			s.MaxDegree = d
		}
	}
	if s.NodeCount > 0 {
		s.AvgDegree = float64(2*s.EdgeCount) / float64(s.NodeCount)
	}
	return s
}

// adjEntry 一条可行的邻接关系：经由 Edge 抵达 To。
type adjEntry struct {
	Edge *Edge
	To   NodeID
}

// adjacentLocked 返回从 from 出发、在 dir 方向上可达的邻接项。
//
// 结果按边 ID 升序，保证遍历顺序确定。调用方必须已持有读锁。
func (g *Graph) adjacentLocked(from NodeID, dir Direction, f *filter) []adjEntry {
	a := g.adj[from]
	if a == nil {
		return nil
	}

	out := make([]adjEntry, 0, len(a.out)+len(a.in))
	appendFrom := func(ids []EdgeID, dedup map[EdgeID]struct{}) {
		for _, eid := range ids {
			if dedup != nil {
				if _, dup := dedup[eid]; dup {
					continue
				}
				dedup[eid] = struct{}{}
			}
			e, ok := g.edges[eid]
			if !ok {
				continue
			}
			if !f.allowRelation(e.Relation) {
				continue
			}
			to := otherEnd(e, from)
			if !f.allowNodeType(g.nodes[to]) {
				continue
			}
			out = append(out, adjEntry{Edge: e, To: to})
		}
	}

	switch dir {
	case DirectionIn:
		appendFrom(a.in, nil)
	case DirectionBoth:
		// 无向边同时登记在 out 与 in 中，双向遍历时必须去重。
		appendFrom(a.out, map[EdgeID]struct{}{})
		dedup := make(map[EdgeID]struct{}, len(a.out))
		for _, eid := range a.out {
			dedup[eid] = struct{}{}
		}
		appendFrom(a.in, dedup)
	default:
		appendFrom(a.out, nil)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Edge.ID < out[j].Edge.ID })
	return out
}

// filter 遍历过滤条件。
type filter struct {
	relations map[RelationType]struct{}
	nodeTypes map[NodeType]struct{}
}

func newFilter(relations []RelationType, nodeTypes []NodeType) *filter {
	f := &filter{}
	if len(relations) > 0 {
		f.relations = make(map[RelationType]struct{}, len(relations))
		for _, r := range relations {
			f.relations[r] = struct{}{}
		}
	}
	if len(nodeTypes) > 0 {
		f.nodeTypes = make(map[NodeType]struct{}, len(nodeTypes))
		for _, t := range nodeTypes {
			f.nodeTypes[t] = struct{}{}
		}
	}
	return f
}

func (f *filter) allowRelation(r RelationType) bool {
	if f == nil || f.relations == nil {
		return true
	}
	_, ok := f.relations[r]
	return ok
}

func (f *filter) allowNodeType(n *Node) bool {
	if f == nil || f.nodeTypes == nil {
		return true
	}
	if n == nil {
		return false
	}
	_, ok := f.nodeTypes[n.Type]
	return ok
}

// ---------------------------------------------------------------- //
// 有序切片辅助
// ---------------------------------------------------------------- //

func insertSorted(s []EdgeID, id EdgeID) []EdgeID {
	i := sort.SearchStrings(s, id)
	if i < len(s) && s[i] == id {
		return s
	}
	s = append(s, "")
	copy(s[i+1:], s[i:])
	s[i] = id
	return s
}

func removeSorted(s []EdgeID, id EdgeID) []EdgeID {
	i := sort.SearchStrings(s, id)
	if i >= len(s) || s[i] != id {
		return s
	}
	return append(s[:i], s[i+1:]...)
}

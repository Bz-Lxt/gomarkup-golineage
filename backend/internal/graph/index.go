package graph

import (
	"fmt"
	"sort"
	"strings"
)

// index 节点二级索引，用于避免高频检索退化为全图扫描。
//
// 索引策略是按访问频次分层的：
//   - 按类型、按属性键、按属性键值：使用倒排 map，O(1) 命中；
//   - 按名称前缀：不建索引，走全量扫描。
//
// 名称前缀之所以例外，是因为维护有序结构在批量导入时会产生 O(n²) 的内存搬移，
// 而 10 万节点的一次前缀扫描仅数毫秒，远在 P95 ≤ 20ms 的基线之内 ——
// 为低频查询付出高昂的写入代价并不划算。
type index struct {
	byType   map[NodeType]map[NodeID]struct{}
	byPropK  map[string]map[NodeID]struct{}
	byPropKV map[string]map[NodeID]struct{}
}

func newIndex() *index {
	return &index{
		byType:   make(map[NodeType]map[NodeID]struct{}),
		byPropK:  make(map[string]map[NodeID]struct{}),
		byPropKV: make(map[string]map[NodeID]struct{}),
	}
}

func (ix *index) reset() {
	ix.byType = make(map[NodeType]map[NodeID]struct{})
	ix.byPropK = make(map[string]map[NodeID]struct{})
	ix.byPropKV = make(map[string]map[NodeID]struct{})
}

// propKVKey 构造属性键值索引项。
//
// 仅对标量值建立键值索引：对象与数组的字符串化结果既冗长又不稳定，
// 建索引收益为负。
func propKVKey(k string, v any) (string, bool) {
	switch tv := v.(type) {
	case string:
		if len(tv) > 128 {
			return "", false
		}
		return k + "\x00" + tv, true
	case bool:
		return fmt.Sprintf("%s\x00%t", k, tv), true
	case float64, float32, int, int32, int64:
		return fmt.Sprintf("%s\x00%v", k, tv), true
	default:
		return "", false
	}
}

func addTo(m map[string]map[NodeID]struct{}, key string, id NodeID) {
	set, ok := m[key]
	if !ok {
		set = make(map[NodeID]struct{})
		m[key] = set
	}
	set[id] = struct{}{}
}

func removeFrom(m map[string]map[NodeID]struct{}, key string, id NodeID) {
	if set, ok := m[key]; ok {
		delete(set, id)
		if len(set) == 0 {
			delete(m, key)
		}
	}
}

// add 将节点登记进各索引。
func (ix *index) add(n *Node) {
	set, ok := ix.byType[n.Type]
	if !ok {
		set = make(map[NodeID]struct{})
		ix.byType[n.Type] = set
	}
	set[n.ID] = struct{}{}

	for k, v := range n.Properties {
		addTo(ix.byPropK, k, n.ID)
		if kv, ok := propKVKey(k, v); ok {
			addTo(ix.byPropKV, kv, n.ID)
		}
	}
}

// remove 将节点从各索引中摘除。
func (ix *index) remove(n *Node) {
	if set, ok := ix.byType[n.Type]; ok {
		delete(set, n.ID)
		if len(set) == 0 {
			delete(ix.byType, n.Type)
		}
	}
	for k, v := range n.Properties {
		removeFrom(ix.byPropK, k, n.ID)
		if kv, ok := propKVKey(k, v); ok {
			removeFrom(ix.byPropKV, kv, n.ID)
		}
	}
}

// SearchOptions 节点检索条件。各条件之间为「与」关系，零值表示不限制。
type SearchOptions struct {
	// NamePrefix 名称前缀，大小写不敏感。
	NamePrefix string
	// Keyword 名称子串匹配，大小写不敏感。
	Keyword string
	// Types 节点类型集合，命中其一即可。
	Types []NodeType
	// PropKey 要求节点必须拥有该属性键。
	PropKey string
	// PropValue 与 PropKey 搭配使用，要求属性值精确相等。
	PropValue any
	// Limit 返回条数上限，<=0 时由调用方的全局上限兜底。
	Limit int
}

// Search 按条件检索节点，结果按名称升序排列以保证输出稳定。
func (g *Graph) Search(opt SearchOptions) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	candidates := g.searchCandidatesLocked(opt)

	prefix := strings.ToLower(strings.TrimSpace(opt.NamePrefix))
	keyword := strings.ToLower(strings.TrimSpace(opt.Keyword))
	typeSet := toTypeSet(opt.Types)

	out := make([]*Node, 0, min(len(candidates), 256))
	for _, n := range candidates {
		if len(typeSet) > 0 {
			if _, ok := typeSet[n.Type]; !ok {
				continue
			}
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(n.Name), prefix) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(n.Name), keyword) {
			continue
		}
		out = append(out, n.Clone())
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})

	if opt.Limit > 0 && len(out) > opt.Limit {
		out = out[:opt.Limit]
	}
	return out
}

// searchCandidatesLocked 挑选代价最低的索引作为候选集起点。
// 调用方必须已持有读锁。
func (g *Graph) searchCandidatesLocked(opt SearchOptions) []*Node {
	// 属性键值索引选择性最高，优先使用。
	if opt.PropKey != "" && opt.PropValue != nil {
		if kv, ok := propKVKey(opt.PropKey, opt.PropValue); ok {
			return g.collectLocked(g.idx.byPropKV[kv])
		}
		// 值类型无法建索引时退化为「拥有该键」再逐一比对。
		ids := g.idx.byPropK[opt.PropKey]
		out := make([]*Node, 0, len(ids))
		for id := range ids {
			if n, ok := g.nodes[id]; ok && fmt.Sprint(n.Properties[opt.PropKey]) == fmt.Sprint(opt.PropValue) {
				out = append(out, n)
			}
		}
		return out
	}
	if opt.PropKey != "" {
		return g.collectLocked(g.idx.byPropK[opt.PropKey])
	}
	// 单一类型过滤时走类型索引。
	if len(opt.Types) == 1 {
		return g.collectLocked(g.idx.byType[opt.Types[0]])
	}
	if len(opt.Types) > 1 {
		merged := make([]*Node, 0, 64)
		for _, t := range opt.Types {
			merged = append(merged, g.collectLocked(g.idx.byType[t])...)
		}
		return merged
	}
	// 无可用索引，退化为全量扫描。
	all := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		all = append(all, n)
	}
	return all
}

func (g *Graph) collectLocked(ids map[NodeID]struct{}) []*Node {
	out := make([]*Node, 0, len(ids))
	for id := range ids {
		if n, ok := g.nodes[id]; ok {
			out = append(out, n)
		}
	}
	return out
}

func toTypeSet(types []NodeType) map[NodeType]struct{} {
	if len(types) == 0 {
		return nil
	}
	s := make(map[NodeType]struct{}, len(types))
	for _, t := range types {
		s[t] = struct{}{}
	}
	return s
}

// TypeCounts 返回各节点类型的数量分布，用于前端统计面板。
func (g *Graph) TypeCounts() map[NodeType]int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make(map[NodeType]int, len(g.idx.byType))
	for t, set := range g.idx.byType {
		out[t] = len(set)
	}
	return out
}

// PropertyKeys 返回图中出现过的全部属性键（升序），用于前端属性编辑器的联想输入。
func (g *Graph) PropertyKeys() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]string, 0, len(g.idx.byPropK))
	for k := range g.idx.byPropK {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

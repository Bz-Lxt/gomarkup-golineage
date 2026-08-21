// Package graph 实现手写的高性能内存图引擎。
//
// 设计要点：
//   - 存储结构为邻接表（出边表 + 入边表双向索引），针对企业资产这类稀疏图优化；
//     邻接矩阵仅在 matrix.go 中对小规模子图按需构建，不作为主存储。
//   - 邻接关系使用「按 ID 升序的切片」而非 map，既省内存，又使遍历顺序稳定 ——
//     Go 的 map 迭代顺序随机，会让 BFS/DFS 结果每次不同，测试无法断言、前端布局抖动。
//   - 本包仅依赖标准库，不感知 HTTP、数据库与业务编排，可独立测试与复用。
package graph

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// NodeID / EdgeID 实体标识，采用 UUID 字符串。
type (
	NodeID = string
	EdgeID = string
)

// NodeType 资产节点类型。
//
// 前四种覆盖「服务器-数据库-应用-接口」IT 资产场景；
// person / account 覆盖金融反欺诈的人际关系网络场景。
type NodeType string

const (
	NodeTypeServer      NodeType = "server"
	NodeTypeDatabase    NodeType = "database"
	NodeTypeApplication NodeType = "application"
	NodeTypeAPI         NodeType = "api"
	NodeTypePerson      NodeType = "person"
	NodeTypeAccount     NodeType = "account"
	NodeTypeMiddleware  NodeType = "middleware"
	NodeTypeStorage     NodeType = "storage"
)

var allNodeTypes = map[NodeType]struct{}{
	NodeTypeServer: {}, NodeTypeDatabase: {}, NodeTypeApplication: {}, NodeTypeAPI: {},
	NodeTypePerson: {}, NodeTypeAccount: {}, NodeTypeMiddleware: {}, NodeTypeStorage: {},
}

// Valid 报告节点类型是否为受支持的枚举值。
func (t NodeType) Valid() bool { _, ok := allNodeTypes[t]; return ok }

// AllNodeTypes 返回全部合法节点类型（供 API 元数据接口下发给前端）。
func AllNodeTypes() []NodeType {
	return []NodeType{
		NodeTypeServer, NodeTypeDatabase, NodeTypeApplication, NodeTypeAPI,
		NodeTypePerson, NodeTypeAccount, NodeTypeMiddleware, NodeTypeStorage,
	}
}

// RelationType 血缘关系类型。
type RelationType string

const (
	RelDeploysOn      RelationType = "deploys_on"      // 部署于
	RelCalls          RelationType = "calls"           // 调用
	RelReadsFrom      RelationType = "reads_from"      // 读取自
	RelWritesTo       RelationType = "writes_to"       // 写入到
	RelDependsOn      RelationType = "depends_on"      // 依赖
	RelOwns           RelationType = "owns"            // 拥有
	RelTransfersTo    RelationType = "transfers_to"    // 转账至
	RelAssociatesWith RelationType = "associates_with" // 关联
)

var allRelationTypes = map[RelationType]struct{}{
	RelDeploysOn: {}, RelCalls: {}, RelReadsFrom: {}, RelWritesTo: {},
	RelDependsOn: {}, RelOwns: {}, RelTransfersTo: {}, RelAssociatesWith: {},
}

// Valid 报告关系类型是否为受支持的枚举值。
func (r RelationType) Valid() bool { _, ok := allRelationTypes[r]; return ok }

// AllRelationTypes 返回全部合法关系类型。
func AllRelationTypes() []RelationType {
	return []RelationType{
		RelDeploysOn, RelCalls, RelReadsFrom, RelWritesTo,
		RelDependsOn, RelOwns, RelTransfersTo, RelAssociatesWith,
	}
}

// Properties 动态属性集合，用于承载 IP、责任人、风险等级等业务字段。
type Properties map[string]any

// 属性键白名单：字母、数字、下划线、连字符与中文，长度 1..64。
// 约束键名可避免 JSONB 中出现异常键，也便于前端表单渲染。
var propKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\p{Han}]{1,64}$`)

const (
	// MaxProperties 单实体最多允许的动态属性数量。
	MaxProperties = 50
	// MaxNameLength 节点名称最大长度。
	MaxNameLength = 128
	// MaxPropValueLength 属性值转为字符串后的最大长度。
	MaxPropValueLength = 2048
)

// Clone 深拷贝属性集合。
//
// 图引擎对外返回的实体一律为副本，防止调用方持有内部指针后并发修改，
// 那会绕过读写锁形成数据竞争。
func (p Properties) Clone() Properties {
	if p == nil {
		return nil
	}
	out := make(Properties, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

// Validate 校验属性集合的键名合法性、数量与值长度上限。
func (p Properties) Validate() error {
	if len(p) > MaxProperties {
		return fmt.Errorf("%w: 属性数量 %d 超过上限 %d", ErrValidation, len(p), MaxProperties)
	}
	for k, v := range p {
		if !propKeyPattern.MatchString(k) {
			return fmt.Errorf("%w: 属性键 %q 非法（仅允许字母/数字/下划线/连字符/中文，长度 1-64）", ErrValidation, k)
		}
		if s, ok := v.(string); ok && len(s) > MaxPropValueLength {
			return fmt.Errorf("%w: 属性 %q 的值长度 %d 超过上限 %d", ErrValidation, k, len(s), MaxPropValueLength)
		}
	}
	return nil
}

// Node 资产节点。
type Node struct {
	ID         NodeID     `json:"id"`
	Name       string     `json:"name"`
	Type       NodeType   `json:"type"`
	Properties Properties `json:"properties,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Clone 深拷贝节点。
func (n *Node) Clone() *Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Properties = n.Properties.Clone()
	return &c
}

// Validate 校验节点字段合法性。
func (n *Node) Validate() error {
	if n == nil {
		return fmt.Errorf("%w: 节点为空", ErrValidation)
	}
	if strings.TrimSpace(n.ID) == "" {
		return fmt.Errorf("%w: 节点 ID 不能为空", ErrValidation)
	}
	name := strings.TrimSpace(n.Name)
	if name == "" {
		return fmt.Errorf("%w: 节点名称不能为空", ErrValidation)
	}
	if len([]rune(name)) > MaxNameLength {
		return fmt.Errorf("%w: 节点名称长度超过上限 %d", ErrValidation, MaxNameLength)
	}
	if !n.Type.Valid() {
		return fmt.Errorf("%w: 节点类型 %q 非法", ErrValidation, n.Type)
	}
	return n.Properties.Validate()
}

// Edge 血缘关系边。
type Edge struct {
	ID         EdgeID       `json:"id"`
	Source     NodeID       `json:"source_id"`
	Target     NodeID       `json:"target_id"`
	Relation   RelationType `json:"relation"`
	Weight     float64      `json:"weight"`
	Directed   bool         `json:"directed"`
	Properties Properties   `json:"properties,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// Clone 深拷贝边。
func (e *Edge) Clone() *Edge {
	if e == nil {
		return nil
	}
	c := *e
	c.Properties = e.Properties.Clone()
	return &c
}

// Validate 校验边字段合法性。
//
// 权重必须为正数：Dijkstra 依赖非负权，负权会使算法给出错误结果，
// 因此在入口处直接拒绝，而不是引入 Bellman-Ford 增加复杂度。
func (e *Edge) Validate() error {
	if e == nil {
		return fmt.Errorf("%w: 边为空", ErrValidation)
	}
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("%w: 边 ID 不能为空", ErrValidation)
	}
	if strings.TrimSpace(e.Source) == "" || strings.TrimSpace(e.Target) == "" {
		return fmt.Errorf("%w: 边的起点与终点不能为空", ErrValidation)
	}
	if e.Source == e.Target {
		return fmt.Errorf("%w: 不允许自环（起点与终点相同）", ErrValidation)
	}
	if !e.Relation.Valid() {
		return fmt.Errorf("%w: 关系类型 %q 非法", ErrValidation, e.Relation)
	}
	if e.Weight <= 0 {
		return fmt.Errorf("%w: 边权重必须为正数，当前 %v", ErrValidation, e.Weight)
	}
	// NaN 无法参与比较，会让优先队列陷入未定义行为。
	if e.Weight != e.Weight {
		return fmt.Errorf("%w: 边权重不能为 NaN", ErrValidation)
	}
	return e.Properties.Validate()
}

// otherEnd 返回边上相对于 from 的另一个端点。自环已被 Validate 禁止，故此处安全。
func otherEnd(e *Edge, from NodeID) NodeID {
	if e.Source == from {
		return e.Target
	}
	return e.Source
}

// Direction 遍历方向。
type Direction uint8

const (
	// DirectionOut 沿边方向前进，语义为「下游 / 影响面」。
	DirectionOut Direction = iota
	// DirectionIn 逆边方向回溯，语义为「上游 / 影响源」。
	DirectionIn
	// DirectionBoth 双向，忽略边的方向性。
	DirectionBoth
)

// ParseDirection 解析方向字符串，空值默认为 out。
func ParseDirection(s string) (Direction, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "out", "downstream":
		return DirectionOut, nil
	case "in", "upstream":
		return DirectionIn, nil
	case "both", "all":
		return DirectionBoth, nil
	default:
		return DirectionOut, fmt.Errorf("%w: 遍历方向 %q 非法（可选 out/in/both）", ErrValidation, s)
	}
}

// String 实现 fmt.Stringer。
func (d Direction) String() string {
	switch d {
	case DirectionIn:
		return "in"
	case DirectionBoth:
		return "both"
	default:
		return "out"
	}
}

// Package seed 生成演示用的初始图谱数据。
//
// 种子数据不是简单地一次性写入终态，而是按时间批次模拟一段真实的架构演进史 ——
// 基础设施先上线、应用逐步接入、中途发生一次架构调整、最后补充风险标注。
// 只有这样，时间轴回溯与拓扑对比功能才有实际内容可展示；
// 若所有事件都发生在同一时刻，时间轴上就只有一个点，回溯功能形同虚设。
package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/internal/timeutil"
	"github.com/alkaid/golineage/pkg/logger"
)

// 场景标识。
const (
	ScenarioAsset = "asset"
	ScenarioFraud = "fraud"
	ScenarioBoth  = "both"
	ScenarioNone  = "none"
)

// builder 在内存中累积事件，最后一次性批量落盘。
type builder struct {
	events []*eventstore.Event
	nodes  map[string]*graph.Node
	edges  map[string]*graph.Edge
	actor  string
}

func newBuilder(actor string) *builder {
	return &builder{
		nodes: make(map[string]*graph.Node),
		edges: make(map[string]*graph.Edge),
		actor: actor,
	}
}

// node 新增一个资产节点。
func (b *builder) node(at time.Time, id, name string, t graph.NodeType, props graph.Properties, reason string) *graph.Node {
	n := &graph.Node{
		ID: id, Name: name, Type: t,
		Properties: props,
		CreatedAt:  at, UpdatedAt: at,
	}
	if n.Properties == nil {
		n.Properties = graph.Properties{}
	}
	ev, err := eventstore.NewNodeEvent(eventstore.EventNodeCreated, n, nil, b.actor, reason, at)
	if err != nil {
		// 种子数据是代码内写死的常量，构造失败说明程序有 bug，不该被静默吞掉。
		panic(fmt.Sprintf("构造种子节点事件失败(%s): %v", id, err))
	}
	b.events = append(b.events, ev)
	b.nodes[id] = n
	return n
}

// edge 新增一条血缘关系。
func (b *builder) edge(at time.Time, id, src, dst string, rel graph.RelationType, weight float64, props graph.Properties, reason string) {
	e := &graph.Edge{
		ID: id, Source: src, Target: dst,
		Relation: rel, Weight: weight, Directed: true,
		Properties: props,
		CreatedAt:  at, UpdatedAt: at,
	}
	if e.Properties == nil {
		e.Properties = graph.Properties{}
	}
	ev, err := eventstore.NewEdgeEvent(eventstore.EventEdgeCreated, e, nil, b.actor, reason, at)
	if err != nil {
		panic(fmt.Sprintf("构造种子关系事件失败(%s): %v", id, err))
	}
	b.events = append(b.events, ev)
	b.edges[id] = e
}

// undirectedEdge 新增一条无向关系，用于人际关联这类无方向语义的场景。
func (b *builder) undirectedEdge(at time.Time, id, src, dst string, rel graph.RelationType, weight float64, props graph.Properties, reason string) {
	e := &graph.Edge{
		ID: id, Source: src, Target: dst,
		Relation: rel, Weight: weight, Directed: false,
		Properties: props,
		CreatedAt:  at, UpdatedAt: at,
	}
	if e.Properties == nil {
		e.Properties = graph.Properties{}
	}
	ev, err := eventstore.NewEdgeEvent(eventstore.EventEdgeCreated, e, nil, b.actor, reason, at)
	if err != nil {
		panic(fmt.Sprintf("构造种子关系事件失败(%s): %v", id, err))
	}
	b.events = append(b.events, ev)
	b.edges[id] = e
}

// updateNode 修改既有节点的属性。
func (b *builder) updateNode(at time.Time, id string, mutate func(*graph.Node), reason string) {
	old, ok := b.nodes[id]
	if !ok {
		panic("种子数据引用了不存在的节点: " + id)
	}
	updated := old.Clone()
	mutate(updated)
	updated.UpdatedAt = at

	ev, err := eventstore.NewNodeEvent(eventstore.EventNodeUpdated, updated, old, b.actor, reason, at)
	if err != nil {
		panic(fmt.Sprintf("构造种子更新事件失败(%s): %v", id, err))
	}
	b.events = append(b.events, ev)
	b.nodes[id] = updated
}

// dropEdge 解除一条既有关系。
func (b *builder) dropEdge(at time.Time, id, reason string) {
	old, ok := b.edges[id]
	if !ok {
		panic("种子数据引用了不存在的关系: " + id)
	}
	ev, err := eventstore.NewEdgeEvent(eventstore.EventEdgeDeleted, nil, old, b.actor, reason, at)
	if err != nil {
		panic(fmt.Sprintf("构造种子删除事件失败(%s): %v", id, err))
	}
	b.events = append(b.events, ev)
	delete(b.edges, id)
}

// Result 种子写入结果。
type Result struct {
	Scenario   string
	EventCount int
	NodeCount  int
	EdgeCount  int
	Earliest   time.Time
	Latest     time.Time
}

// Apply 按场景生成并写入种子事件。
//
// 只有事件日志为空时才应调用，否则会与既有数据混杂。
func Apply(ctx context.Context, store eventstore.Store, scenario string) (*Result, error) {
	if scenario == ScenarioNone {
		return &Result{Scenario: scenario}, nil
	}

	now := timeutil.Now().Truncate(time.Minute)
	b := newBuilder("seed-bot")

	switch scenario {
	case ScenarioAsset:
		buildAssetScenario(b, now)
	case ScenarioFraud:
		buildFraudScenario(b, now)
	case ScenarioBoth:
		buildAssetScenario(b, now)
		buildFraudScenario(b, now)
	default:
		return nil, fmt.Errorf("未知的种子场景: %s", scenario)
	}

	if len(b.events) == 0 {
		return &Result{Scenario: scenario}, nil
	}
	if err := store.Append(ctx, b.events); err != nil {
		return nil, fmt.Errorf("写入种子事件失败: %w", err)
	}

	res := &Result{
		Scenario:   scenario,
		EventCount: len(b.events),
		NodeCount:  len(b.nodes),
		EdgeCount:  len(b.edges),
		Earliest:   b.events[0].OccurredAt,
		Latest:     b.events[len(b.events)-1].OccurredAt,
	}
	logger.Info("种子数据已写入",
		"scenario", scenario, "events", res.EventCount,
		"nodes", res.NodeCount, "edges", res.EdgeCount,
		"from", timeutil.Format(res.Earliest), "to", timeutil.Format(res.Latest))
	return res, nil
}

// buildAssetScenario 构建 IT 资产血缘场景：服务器 — 数据库 — 应用 — 接口。
func buildAssetScenario(b *builder, now time.Time) {
	day := 24 * time.Hour
	t60 := now.Add(-60 * day) // 基础设施上线
	t45 := now.Add(-45 * day) // 中间件与数据库
	t30 := now.Add(-30 * day) // 核心服务接入
	t20 := now.Add(-20 * day) // 网关与前台应用
	t10 := now.Add(-10 * day) // 新增报表链路
	t5 := now.Add(-5 * day)   // 架构调整
	t2 := now.Add(-2 * day)   // 风险评级

	// —— 第一阶段：物理基础设施 ——
	b.node(t60, "srv-app-01", "应用服务器 A", graph.NodeTypeServer, graph.Properties{
		"ip": "10.10.1.11", "机房": "上海-金桥", "规格": "16C64G", "owner": "运维-王强",
	}, "机房初始交付")
	b.node(t60, "srv-app-02", "应用服务器 B", graph.NodeTypeServer, graph.Properties{
		"ip": "10.10.1.12", "机房": "上海-金桥", "规格": "16C64G", "owner": "运维-王强",
	}, "机房初始交付")
	b.node(t60, "srv-db-01", "数据库服务器主", graph.NodeTypeServer, graph.Properties{
		"ip": "10.10.2.11", "机房": "上海-金桥", "规格": "32C128G", "owner": "DBA-李敏",
	}, "机房初始交付")
	b.node(t60, "srv-db-02", "数据库服务器备", graph.NodeTypeServer, graph.Properties{
		"ip": "10.10.2.12", "机房": "北京-亦庄", "规格": "32C128G", "owner": "DBA-李敏",
	}, "异地灾备节点交付")

	// —— 第二阶段：数据与中间件 ——
	b.node(t45, "db-user", "用户主库", graph.NodeTypeDatabase, graph.Properties{
		"engine": "PostgreSQL 17", "ip": "10.10.2.11", "owner": "DBA-李敏", "备份策略": "每日全量+增量",
	}, "用户体系建设")
	b.node(t45, "db-order", "订单库", graph.NodeTypeDatabase, graph.Properties{
		"engine": "PostgreSQL 17", "ip": "10.10.2.11", "owner": "DBA-李敏",
	}, "交易体系建设")
	b.node(t45, "db-payment", "支付库", graph.NodeTypeDatabase, graph.Properties{
		"engine": "PostgreSQL 17", "ip": "10.10.2.12", "owner": "DBA-李敏", "合规等级": "PCI-DSS",
	}, "支付体系建设")
	b.node(t45, "cache-redis", "Redis 集群", graph.NodeTypeMiddleware, graph.Properties{
		"ip": "10.10.3.11", "版本": "7.4", "owner": "运维-王强",
	}, "缓存层建设")
	b.node(t45, "mq-kafka", "Kafka 消息总线", graph.NodeTypeMiddleware, graph.Properties{
		"ip": "10.10.3.21", "版本": "3.8", "分区数": 24, "owner": "运维-王强",
	}, "异步解耦建设")
	b.node(t45, "oss-static", "对象存储", graph.NodeTypeStorage, graph.Properties{
		"endpoint": "oss-cn-shanghai", "容量": "20TB", "owner": "运维-王强",
	}, "静态资源托管")

	b.edge(t45, "e-db-user-srv", "db-user", "srv-db-01", graph.RelDeploysOn, 1, nil, "数据库实例部署")
	b.edge(t45, "e-db-order-srv", "db-order", "srv-db-01", graph.RelDeploysOn, 1, nil, "数据库实例部署")
	b.edge(t45, "e-db-pay-srv", "db-payment", "srv-db-02", graph.RelDeploysOn, 1, nil, "支付库独立部署")
	b.edge(t45, "e-cache-srv", "cache-redis", "srv-app-02", graph.RelDeploysOn, 1, nil, "缓存部署")
	b.edge(t45, "e-mq-srv", "mq-kafka", "srv-app-02", graph.RelDeploysOn, 1, nil, "消息总线部署")

	// —— 第三阶段：核心微服务 ——
	b.node(t30, "svc-user", "用户服务", graph.NodeTypeApplication, graph.Properties{
		"language": "Go", "副本数": 4, "owner": "研发-张伟", "SLA": "99.95%",
	}, "微服务化改造")
	b.node(t30, "svc-order", "订单服务", graph.NodeTypeApplication, graph.Properties{
		"language": "Go", "副本数": 6, "owner": "研发-张伟", "SLA": "99.99%",
	}, "微服务化改造")
	b.node(t30, "svc-payment", "支付服务", graph.NodeTypeApplication, graph.Properties{
		"language": "Java", "副本数": 4, "owner": "研发-陈静", "SLA": "99.99%",
	}, "微服务化改造")
	b.node(t30, "svc-notify", "通知服务", graph.NodeTypeApplication, graph.Properties{
		"language": "Go", "副本数": 2, "owner": "研发-陈静",
	}, "微服务化改造")

	b.edge(t30, "e-svc-user-srv", "svc-user", "srv-app-01", graph.RelDeploysOn, 1, nil, "服务部署")
	b.edge(t30, "e-svc-order-srv", "svc-order", "srv-app-01", graph.RelDeploysOn, 1, nil, "服务部署")
	b.edge(t30, "e-svc-pay-srv", "svc-payment", "srv-app-02", graph.RelDeploysOn, 1, nil, "服务部署")
	b.edge(t30, "e-svc-notify-srv", "svc-notify", "srv-app-02", graph.RelDeploysOn, 1, nil, "服务部署")

	b.edge(t30, "e-user-db", "svc-user", "db-user", graph.RelReadsFrom, 1.2,
		graph.Properties{"平均延迟ms": 4}, "读取用户资料")
	b.edge(t30, "e-user-dbw", "svc-user", "db-user", graph.RelWritesTo, 2.5,
		graph.Properties{"平均延迟ms": 12}, "写入用户变更")
	b.edge(t30, "e-order-db", "svc-order", "db-order", graph.RelReadsFrom, 1.5,
		graph.Properties{"平均延迟ms": 6}, "读取订单")
	b.edge(t30, "e-order-dbw", "svc-order", "db-order", graph.RelWritesTo, 3,
		graph.Properties{"平均延迟ms": 18}, "写入订单")
	b.edge(t30, "e-pay-db", "svc-payment", "db-payment", graph.RelWritesTo, 3.5,
		graph.Properties{"平均延迟ms": 22, "合规": "需审计"}, "写入支付流水")
	b.edge(t30, "e-user-cache", "svc-user", "cache-redis", graph.RelDependsOn, 0.5, nil, "会话缓存")
	b.edge(t30, "e-order-user", "svc-order", "svc-user", graph.RelCalls, 2,
		graph.Properties{"QPS": 800}, "下单校验用户")
	b.edge(t30, "e-order-pay", "svc-order", "svc-payment", graph.RelCalls, 2.5,
		graph.Properties{"QPS": 400}, "发起支付")
	b.edge(t30, "e-pay-mq", "svc-payment", "mq-kafka", graph.RelWritesTo, 1, nil, "支付结果投递")
	b.edge(t30, "e-notify-mq", "svc-notify", "mq-kafka", graph.RelReadsFrom, 1, nil, "消费支付结果")

	// —— 第四阶段：网关与前台 ——
	b.node(t20, "api-gateway", "统一 API 网关", graph.NodeTypeAPI, graph.Properties{
		"ip": "10.10.0.10", "限流QPS": 20000, "owner": "研发-张伟",
	}, "统一入口治理")
	b.node(t20, "api-openapi", "开放平台接口", graph.NodeTypeAPI, graph.Properties{
		"对外": true, "认证方式": "OAuth2", "owner": "研发-陈静",
	}, "对外能力开放")
	b.node(t20, "app-portal", "客户门户", graph.NodeTypeApplication, graph.Properties{
		"framework": "Vue 3", "owner": "研发-刘洋", "日活": 52000,
	}, "前台上线")
	b.node(t20, "app-admin", "运营后台", graph.NodeTypeApplication, graph.Properties{
		"framework": "Vue 3", "owner": "研发-刘洋", "内部系统": true,
	}, "后台上线")
	b.node(t20, "app-mobile", "移动端 App", graph.NodeTypeApplication, graph.Properties{
		"platform": "iOS/Android", "owner": "研发-刘洋", "日活": 130000,
	}, "移动端上线")

	b.edge(t20, "e-portal-gw", "app-portal", "api-gateway", graph.RelCalls, 1, nil, "前台经网关访问")
	b.edge(t20, "e-admin-gw", "app-admin", "api-gateway", graph.RelCalls, 1, nil, "后台经网关访问")
	b.edge(t20, "e-mobile-gw", "app-mobile", "api-gateway", graph.RelCalls, 1, nil, "移动端经网关访问")
	b.edge(t20, "e-gw-user", "api-gateway", "svc-user", graph.RelCalls, 1, nil, "路由至用户服务")
	b.edge(t20, "e-gw-order", "api-gateway", "svc-order", graph.RelCalls, 1, nil, "路由至订单服务")
	b.edge(t20, "e-openapi-gw", "api-openapi", "api-gateway", graph.RelDependsOn, 1, nil, "开放接口复用网关")
	b.edge(t20, "e-portal-oss", "app-portal", "oss-static", graph.RelReadsFrom, 0.8, nil, "加载静态资源")

	// —— 第五阶段：报表链路 ——
	b.node(t10, "db-analytics", "分析库", graph.NodeTypeDatabase, graph.Properties{
		"engine": "ClickHouse", "ip": "10.10.2.31", "owner": "数据-赵磊",
	}, "数据分析体系建设")
	b.node(t10, "svc-report", "报表服务", graph.NodeTypeApplication, graph.Properties{
		"language": "Python", "副本数": 2, "owner": "数据-赵磊",
	}, "数据分析体系建设")

	b.edge(t10, "e-report-analytics", "svc-report", "db-analytics", graph.RelReadsFrom, 2, nil, "报表查询")
	b.edge(t10, "e-report-mq", "svc-report", "mq-kafka", graph.RelReadsFrom, 1, nil, "消费业务事件")
	b.edge(t10, "e-admin-report", "app-admin", "svc-report", graph.RelCalls, 1.5, nil, "后台查看报表")

	// 这条直连是本演示的关键：报表服务早期为图快而直连订单主库，
	// 后续会被治理掉，成为时间轴回溯与拓扑对比最直观的例子。
	b.edge(t10, "e-report-order-db", "svc-report", "db-order", graph.RelReadsFrom, 4,
		graph.Properties{"风险": "直连业务主库", "平均延迟ms": 260}, "报表临时直连订单库取数")

	// —— 第六阶段：架构治理 ——
	b.dropEdge(t5, "e-report-order-db",
		"治理决策：报表服务不再直连订单主库，改由 Kafka 事件流同步至分析库，避免慢查询拖垮交易链路")
	b.edge(t5, "e-analytics-mq", "db-analytics", "mq-kafka", graph.RelReadsFrom, 1,
		graph.Properties{"同步方式": "CDC"}, "改由事件流入湖")

	// —— 第七阶段：风险评级 ——
	b.updateNode(t2, "db-payment", func(n *graph.Node) {
		n.Properties["risk_level"] = "high"
		n.Properties["风险说明"] = "承载资金流水，故障影响面覆盖全部交易"
	}, "季度安全评级：核心资金库标记为高风险")
	b.updateNode(t2, "db-user", func(n *graph.Node) {
		n.Properties["risk_level"] = "high"
		n.Properties["风险说明"] = "存储个人敏感信息，受个人信息保护法约束"
	}, "季度安全评级：个人信息库标记为高风险")
	b.updateNode(t2, "api-gateway", func(n *graph.Node) {
		n.Properties["risk_level"] = "high"
		n.Properties["风险说明"] = "全站唯一入口，单点故障将导致全量业务不可用"
	}, "季度安全评级：唯一入口标记为高风险")
	b.updateNode(t2, "svc-order", func(n *graph.Node) {
		n.Properties["risk_level"] = "medium"
	}, "季度安全评级")
	b.updateNode(t2, "mq-kafka", func(n *graph.Node) {
		n.Properties["risk_level"] = "medium"
	}, "季度安全评级")
	b.updateNode(t2, "oss-static", func(n *graph.Node) {
		n.Properties["risk_level"] = "low"
	}, "季度安全评级")
}

// buildFraudScenario 构建金融反欺诈人际关系网络场景。
//
// 刻意埋入一个环形资金回路（P1 → P2 → P3 → P1），
// 这是典型的洗钱结构，用于演示环路检测与全路径枚举的实际价值。
func buildFraudScenario(b *builder, now time.Time) {
	day := 24 * time.Hour
	t40 := now.Add(-40 * day)
	t25 := now.Add(-25 * day)
	t12 := now.Add(-12 * day)
	t3 := now.Add(-3 * day)

	// —— 自然人 ——
	people := []struct {
		id, name, phone, city string
		risk                  string
	}{
		{"p-001", "陈国栋", "138****2011", "上海", "high"},
		{"p-002", "黄丽华", "139****3022", "杭州", "medium"},
		{"p-003", "吴建平", "137****4033", "深圳", "high"},
		{"p-004", "周雅琴", "136****5044", "上海", "low"},
		{"p-005", "郑文博", "135****6055", "南京", "medium"},
		{"p-006", "孙晓峰", "133****7066", "广州", "low"},
	}
	for _, p := range people {
		b.node(t40, p.id, p.name, graph.NodeTypePerson, graph.Properties{
			"手机号": p.phone, "常驻城市": p.city, "risk_level": p.risk,
			"实名认证": true,
		}, "客户实名建档")
	}

	// —— 银行账户 ——
	accounts := []struct {
		id, name, bank, owner string
		balance               float64
	}{
		{"a-001", "尾号 8801 账户", "工商银行", "p-001", 128000},
		{"a-002", "尾号 8802 账户", "招商银行", "p-001", 43500},
		{"a-003", "尾号 8803 账户", "建设银行", "p-002", 96200},
		{"a-004", "尾号 8804 账户", "招商银行", "p-003", 271000},
		{"a-005", "尾号 8805 账户", "农业银行", "p-004", 15800},
		{"a-006", "尾号 8806 账户", "工商银行", "p-005", 62400},
		{"a-007", "尾号 8807 账户", "中国银行", "p-006", 8900},
		{"a-008", "尾号 8808 账户", "招商银行", "p-003", 340000},
	}
	for _, a := range accounts {
		b.node(t40, a.id, a.name, graph.NodeTypeAccount, graph.Properties{
			"开户行": a.bank, "余额": a.balance, "账户状态": "正常",
		}, "账户开立")
		b.edge(t40, "e-own-"+a.id, a.owner, a.id, graph.RelOwns, 1, nil, "账户实名绑定")
	}

	// —— 社会关系 ——
	b.undirectedEdge(t25, "e-rel-1-2", "p-001", "p-002", graph.RelAssociatesWith, 1,
		graph.Properties{"关系": "同事", "共同居住地": false}, "关系图谱补录")
	b.undirectedEdge(t25, "e-rel-2-3", "p-002", "p-003", graph.RelAssociatesWith, 1,
		graph.Properties{"关系": "亲属"}, "关系图谱补录")
	b.undirectedEdge(t25, "e-rel-3-5", "p-003", "p-005", graph.RelAssociatesWith, 2,
		graph.Properties{"关系": "同一设备登录"}, "设备指纹关联")
	b.undirectedEdge(t25, "e-rel-4-6", "p-004", "p-006", graph.RelAssociatesWith, 1,
		graph.Properties{"关系": "同一收货地址"}, "地址关联")
	b.undirectedEdge(t25, "e-rel-1-4", "p-001", "p-004", graph.RelAssociatesWith, 3,
		graph.Properties{"关系": "同一 WiFi 网络"}, "网络环境关联")

	// —— 资金流动：构成 a-001 → a-003 → a-004 → a-001 的环形回路 ——
	b.edge(t12, "e-tx-1", "a-001", "a-003", graph.RelTransfersTo, 1,
		graph.Properties{"金额": 98000, "次数": 3, "备注": "货款"}, "资金流水入图")
	b.edge(t12, "e-tx-2", "a-003", "a-004", graph.RelTransfersTo, 1,
		graph.Properties{"金额": 95500, "次数": 2, "备注": "借款"}, "资金流水入图")
	b.edge(t12, "e-tx-3", "a-004", "a-001", graph.RelTransfersTo, 1,
		graph.Properties{"金额": 94000, "次数": 4, "备注": "还款"}, "资金流水入图")
	b.edge(t12, "e-tx-4", "a-004", "a-008", graph.RelTransfersTo, 2,
		graph.Properties{"金额": 210000, "次数": 1}, "资金流水入图")
	b.edge(t12, "e-tx-5", "a-006", "a-002", graph.RelTransfersTo, 3,
		graph.Properties{"金额": 12000, "次数": 8, "备注": "小额多次"}, "资金流水入图")
	b.edge(t12, "e-tx-6", "a-005", "a-007", graph.RelTransfersTo, 4,
		graph.Properties{"金额": 3200, "次数": 2}, "资金流水入图")
	b.edge(t12, "e-tx-7", "a-002", "a-005", graph.RelTransfersTo, 3,
		graph.Properties{"金额": 8600, "次数": 5}, "资金流水入图")

	// —— 风控命中：三日前的模型输出 ——
	b.updateNode(t3, "p-001", func(n *graph.Node) {
		n.Properties["风控标记"] = "疑似资金回流核心节点"
		n.Properties["模型评分"] = 92
	}, "反欺诈模型命中：账户间存在闭环资金流动")
	b.updateNode(t3, "p-003", func(n *graph.Node) {
		n.Properties["风控标记"] = "大额可疑转出"
		n.Properties["模型评分"] = 87
	}, "反欺诈模型命中：单笔大额转出且对手方关联度高")
	b.updateNode(t3, "a-004", func(n *graph.Node) {
		n.Properties["账户状态"] = "受限"
		n.Properties["risk_level"] = "high"
	}, "风控处置：账户交易受限")
}

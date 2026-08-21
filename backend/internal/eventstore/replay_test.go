package eventstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alkaid/golineage/internal/graph"
)

func testNode(id, name string, t graph.NodeType, props graph.Properties) *graph.Node {
	return &graph.Node{ID: id, Name: name, Type: t, Properties: props}
}

func testEdge(id, src, dst string, rel graph.RelationType, w float64) *graph.Edge {
	return &graph.Edge{ID: id, Source: src, Target: dst, Relation: rel, Weight: w, Directed: true}
}

// appendNode 构造并追加一条节点事件。
func appendNode(t *testing.T, s Store, typ EventType, n, before *graph.Node, at time.Time) {
	t.Helper()
	e, err := NewNodeEvent(typ, n, before, "tester", "单元测试", at)
	if err != nil {
		t.Fatalf("构造节点事件失败: %v", err)
	}
	if err := s.Append(t.Context(), []*Event{e}); err != nil {
		t.Fatalf("追加事件失败: %v", err)
	}
}

func appendEdge(t *testing.T, s Store, typ EventType, e, before *graph.Edge, at time.Time) {
	t.Helper()
	ev, err := NewEdgeEvent(typ, e, before, "tester", "单元测试", at)
	if err != nil {
		t.Fatalf("构造关系事件失败: %v", err)
	}
	if err := s.Append(t.Context(), []*Event{ev}); err != nil {
		t.Fatalf("追加事件失败: %v", err)
	}
}

var baseTime = time.Date(2026, 8, 21, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))

// seedTimeline 写入一条有明确时间分期的变更流水：
//
//	T1  创建 app / db，建立 app -reads_from-> db
//	T2  修改 app 属性（新增责任人）
//	T3  解除 app -> db 关系（模拟「A 应用不再调用 B 数据库」）
func seedTimeline(t *testing.T, s Store) (t1, t2, t3 time.Time) {
	t.Helper()
	t1 = baseTime
	t2 = baseTime.Add(time.Hour)
	t3 = baseTime.Add(2 * time.Hour)

	app := testNode("app", "订单应用", graph.NodeTypeApplication, graph.Properties{"owner": "张三"})
	db := testNode("db", "订单库", graph.NodeTypeDatabase, nil)
	edge := testEdge("e1", "app", "db", graph.RelReadsFrom, 1)

	appendNode(t, s, EventNodeCreated, app, nil, t1)
	appendNode(t, s, EventNodeCreated, db, nil, t1)
	appendEdge(t, s, EventEdgeCreated, edge, nil, t1)

	updated := app.Clone()
	updated.Properties["risk_level"] = "high"
	appendNode(t, s, EventNodeUpdated, updated, app, t2)

	appendEdge(t, s, EventEdgeDeleted, nil, edge, t3)
	return t1, t2, t3
}

func TestApplyNodeLifecycle(t *testing.T) {
	g := graph.New(graph.DefaultLimits())
	n := testNode("a", "资产A", graph.NodeTypeServer, graph.Properties{"ip": "10.0.0.1"})

	created, _ := NewNodeEvent(EventNodeCreated, n, nil, "tester", "", baseTime)
	if err := Apply(g, created); err != nil {
		t.Fatalf("应用创建事件失败: %v", err)
	}
	if g.NodeCount() != 1 {
		t.Fatalf("期望 1 个节点，实际 %d", g.NodeCount())
	}

	updated := n.Clone()
	updated.Name = "资产A改"
	upEvent, _ := NewNodeEvent(EventNodeUpdated, updated, n, "tester", "", baseTime)
	if err := Apply(g, upEvent); err != nil {
		t.Fatalf("应用更新事件失败: %v", err)
	}
	got, _ := g.GetNode("a")
	if got.Name != "资产A改" {
		t.Errorf("期望名称已更新，实际 %s", got.Name)
	}

	delEvent, _ := NewNodeEvent(EventNodeDeleted, nil, n, "tester", "", baseTime)
	if err := Apply(g, delEvent); err != nil {
		t.Fatalf("应用删除事件失败: %v", err)
	}
	if g.NodeCount() != 0 {
		t.Errorf("期望删除后为空图，实际 %d 个节点", g.NodeCount())
	}
}

// TestApplyDeleteMissingIsIdempotent 重复删除应被容忍。
// 删除节点会级联产生关系删除事件，这些关系可能已被更早的事件移除。
func TestApplyDeleteMissingIsIdempotent(t *testing.T) {
	g := graph.New(graph.DefaultLimits())

	del, _ := NewNodeEvent(EventNodeDeleted, nil, testNode("ghost", "幽灵", graph.NodeTypeServer, nil), "t", "", baseTime)
	if err := Apply(g, del); err != nil {
		t.Errorf("删除不存在的节点应被容忍，实际报错: %v", err)
	}

	delEdge, _ := NewEdgeEvent(EventEdgeDeleted, nil, testEdge("ghost-e", "a", "b", graph.RelCalls, 1), "t", "", baseTime)
	if err := Apply(g, delEdge); err != nil {
		t.Errorf("删除不存在的关系应被容忍，实际报错: %v", err)
	}
}

// TestApplyRejectsCorruptPayload 载荷损坏必须中止重放而非静默跳过。
// 静默跳过会构建出缺失关系的拓扑，而使用者完全无从察觉。
func TestApplyRejectsCorruptPayload(t *testing.T) {
	g := graph.New(graph.DefaultLimits())

	cases := []struct {
		name string
		ev   *Event
	}{
		{
			name: "载荷不是合法 JSON",
			ev: &Event{
				Seq: 1, Type: EventNodeCreated, EntityType: EntityNode,
				EntityID: "a", Payload: json.RawMessage(`{"id":`),
			},
		},
		{
			name: "节点类型非法",
			ev: &Event{
				Seq: 2, Type: EventNodeCreated, EntityType: EntityNode,
				EntityID: "a", Payload: json.RawMessage(`{"id":"a","name":"A","type":"不存在的类型"}`),
			},
		},
		{
			name: "载荷 ID 与事件实体 ID 不一致",
			ev: &Event{
				Seq: 3, Type: EventNodeCreated, EntityType: EntityNode,
				EntityID: "a", Payload: json.RawMessage(`{"id":"b","name":"B","type":"server"}`),
			},
		},
		{
			name: "边权重为负",
			ev: &Event{
				Seq: 4, Type: EventEdgeCreated, EntityType: EntityEdge,
				EntityID: "e", Payload: json.RawMessage(`{"id":"e","source_id":"a","target_id":"b","relation":"calls","weight":-1}`),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Apply(g, tc.ev); err == nil {
				t.Fatal("损坏的事件必须报错，不能静默跳过")
			}
		})
	}
}

func TestEventValidate(t *testing.T) {
	cases := []struct {
		name    string
		ev      *Event
		wantErr bool
	}{
		{"合法事件", &Event{Type: EventNodeDeleted, EntityType: EntityNode, EntityID: "a"}, false},
		{"未知事件类型", &Event{Type: "bogus", EntityType: EntityNode, EntityID: "a"}, true},
		{"实体 ID 为空", &Event{Type: EventNodeDeleted, EntityType: EntityNode, EntityID: "  "}, true},
		{"类型与实体不匹配", &Event{Type: EventEdgeCreated, EntityType: EntityNode, EntityID: "a"}, true},
		{"创建事件缺少载荷", &Event{Type: EventNodeCreated, EntityType: EntityNode, EntityID: "a"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ev.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("期望出错=%v，实际 err=%v", tc.wantErr, err)
			}
		})
	}
}

// TestReplayConsistency 重放必须精确重建与写入时一致的拓扑。
// 这是「进程重启后数据不丢」这一承诺的直接验证。
func TestReplayConsistency(t *testing.T) {
	store := NewMemoryStore()
	seedTimeline(t, store)

	live := graph.New(graph.DefaultLimits())
	stats, err := ReplayLive(t.Context(), store, live)
	if err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if stats.EventsApplied != 5 {
		t.Errorf("期望应用 5 条事件，实际 %d", stats.EventsApplied)
	}
	if live.NodeCount() != 2 {
		t.Errorf("期望 2 个节点，实际 %d", live.NodeCount())
	}
	// e1 已在 T3 被解除，最终拓扑不应有边。
	if live.EdgeCount() != 0 {
		t.Errorf("关系已解除，期望 0 条边，实际 %d", live.EdgeCount())
	}
	app, err := live.GetNode("app")
	if err != nil {
		t.Fatalf("GetNode 失败: %v", err)
	}
	if app.Properties["risk_level"] != "high" {
		t.Errorf("T2 的属性更新未生效: %v", app.Properties)
	}

	// 再次重放到一张全新的图上，结果必须完全相同（重放具有确定性）。
	second := graph.New(graph.DefaultLimits())
	if _, err := ReplayLive(t.Context(), store, second); err != nil {
		t.Fatalf("二次重放失败: %v", err)
	}
	assertSameTopology(t, live, second)
}

// TestReplayIsIdempotentOnDirtyGraph 对已有内容的图重放，必须先清空再重建，
// 否则残留状态会污染结果。
func TestReplayIsIdempotentOnDirtyGraph(t *testing.T) {
	store := NewMemoryStore()
	seedTimeline(t, store)

	dirty := graph.New(graph.DefaultLimits())
	if err := dirty.AddNode(testNode("stale", "残留节点", graph.NodeTypeServer, nil)); err != nil {
		t.Fatal(err)
	}

	if _, err := ReplayLive(t.Context(), store, dirty); err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if dirty.HasNode("stale") {
		t.Error("重放前应清空原有内容，残留节点不应存在")
	}
	if dirty.NodeCount() != 2 {
		t.Errorf("期望 2 个节点，实际 %d", dirty.NodeCount())
	}
}

// TestReplayCascadeDelete 删除节点产生的级联关系事件必须让重放结果无悬空边。
func TestReplayCascadeDelete(t *testing.T) {
	store := NewMemoryStore()
	ctx := t.Context()

	a := testNode("a", "应用A", graph.NodeTypeApplication, nil)
	b := testNode("b", "数据库B", graph.NodeTypeDatabase, nil)
	e := testEdge("ab", "a", "b", graph.RelReadsFrom, 1)

	appendNode(t, store, EventNodeCreated, a, nil, baseTime)
	appendNode(t, store, EventNodeCreated, b, nil, baseTime)
	appendEdge(t, store, EventEdgeCreated, e, nil, baseTime)

	// 级联删除：关系事件在前、节点事件在后，重放时才不会留下悬空边。
	edgeDel, _ := NewEdgeEvent(EventEdgeDeleted, nil, e, "t", "应用下线", baseTime.Add(time.Hour))
	nodeDel, _ := NewNodeEvent(EventNodeDeleted, nil, a, "t", "应用下线", baseTime.Add(time.Hour))
	if err := store.Append(ctx, []*Event{edgeDel, nodeDel}); err != nil {
		t.Fatalf("追加级联事件失败: %v", err)
	}

	g := graph.New(graph.DefaultLimits())
	if _, err := ReplayLive(ctx, store, g); err != nil {
		t.Fatalf("重放失败: %v", err)
	}
	if g.NodeCount() != 1 || g.EdgeCount() != 0 {
		t.Fatalf("期望剩 1 个节点 0 条边，实际 %d 节点 %d 边", g.NodeCount(), g.EdgeCount())
	}
	if g.HasNode("a") {
		t.Error("节点 a 应已被删除")
	}
}

func TestReplayWithCheckpoint(t *testing.T) {
	store := NewMemoryStore()
	ctx := t.Context()
	seedTimeline(t, store)

	// 先重放一次，再据此落检查点。
	g := graph.New(graph.DefaultLimits())
	stats, err := ReplayLive(ctx, store, g)
	if err != nil {
		t.Fatalf("首次重放失败: %v", err)
	}
	snap := g.Snapshot()
	if err := store.SaveCheckpoint(ctx, &Checkpoint{
		LastSeq: stats.LastSeq, NodeCount: len(snap.Nodes), EdgeCount: len(snap.Edges), Snapshot: snap,
	}); err != nil {
		t.Fatalf("保存检查点失败: %v", err)
	}

	// 检查点之后再产生一条增量事件。
	srv := testNode("srv", "宿主机", graph.NodeTypeServer, nil)
	appendNode(t, store, EventNodeCreated, srv, nil, baseTime.Add(3*time.Hour))

	// 从检查点恢复时只应重放那 1 条增量。
	fresh := graph.New(graph.DefaultLimits())
	s2, err := ReplayLive(ctx, store, fresh)
	if err != nil {
		t.Fatalf("检查点重放失败: %v", err)
	}
	if !s2.UsedCheckpoint {
		t.Error("存在检查点时应标记 UsedCheckpoint")
	}
	if s2.EventsApplied != 1 {
		t.Errorf("期望只重放 1 条增量事件，实际 %d", s2.EventsApplied)
	}
	if fresh.NodeCount() != 3 {
		t.Errorf("期望 3 个节点，实际 %d", fresh.NodeCount())
	}

	// 检查点只是加速手段：全量重放必须得到同样的结果。
	full := graph.New(graph.DefaultLimits())
	if _, err := replay(ctx, store, full, nil, false); err != nil {
		t.Fatalf("全量重放失败: %v", err)
	}
	assertSameTopology(t, fresh, full)
}

func TestMaybeCheckpoint(t *testing.T) {
	store := NewMemoryStore()
	ctx := t.Context()
	seedTimeline(t, store)

	g := graph.New(graph.DefaultLimits())
	stats, _ := ReplayLive(ctx, store, g)

	t.Run("未达阈值不落盘", func(t *testing.T) {
		MaybeCheckpoint(ctx, store, g, stats.LastSeq, 1000)
		cp, _ := store.LatestCheckpoint(ctx, nil)
		if cp != nil {
			t.Errorf("累计 %d 条事件未达阈值 1000，不应落检查点", stats.LastSeq)
		}
	})

	t.Run("达到阈值落盘", func(t *testing.T) {
		MaybeCheckpoint(ctx, store, g, stats.LastSeq, 2)
		cp, _ := store.LatestCheckpoint(ctx, nil)
		if cp == nil {
			t.Fatal("应已落检查点")
		}
		if cp.LastSeq != stats.LastSeq {
			t.Errorf("检查点 LastSeq 期望 %d，实际 %d", stats.LastSeq, cp.LastSeq)
		}
	})

	t.Run("阈值为零时关闭机制", func(t *testing.T) {
		s := NewMemoryStore()
		MaybeCheckpoint(ctx, s, g, 9999, 0)
		cp, _ := s.LatestCheckpoint(ctx, nil)
		if cp != nil {
			t.Error("interval<=0 应关闭检查点机制")
		}
	})
}

// TestSnapshotAtTimeTravel 时间轴回溯：不同时刻应还原出不同的历史拓扑。
func TestSnapshotAtTimeTravel(t *testing.T) {
	store := NewMemoryStore()
	t1, t2, t3 := seedTimeline(t, store)
	ctx := t.Context()

	t.Run("T1 关系仍然存在", func(t *testing.T) {
		g, _, err := SnapshotAt(ctx, store, t1, graph.DefaultLimits())
		if err != nil {
			t.Fatalf("SnapshotAt 失败: %v", err)
		}
		if g.NodeCount() != 2 || g.EdgeCount() != 1 {
			t.Fatalf("T1 期望 2 节点 1 边，实际 %d 节点 %d 边", g.NodeCount(), g.EdgeCount())
		}
		app, _ := g.GetNode("app")
		if _, has := app.Properties["risk_level"]; has {
			t.Error("T1 时刻还没有 risk_level 属性")
		}
	})

	t.Run("T2 属性已更新但关系仍在", func(t *testing.T) {
		g, _, err := SnapshotAt(ctx, store, t2, graph.DefaultLimits())
		if err != nil {
			t.Fatalf("SnapshotAt 失败: %v", err)
		}
		app, _ := g.GetNode("app")
		if app.Properties["risk_level"] != "high" {
			t.Errorf("T2 应已有 risk_level=high，实际 %v", app.Properties)
		}
		if g.EdgeCount() != 1 {
			t.Errorf("T2 关系尚未解除，期望 1 条边，实际 %d", g.EdgeCount())
		}
	})

	t.Run("T3 关系已解除", func(t *testing.T) {
		g, _, err := SnapshotAt(ctx, store, t3, graph.DefaultLimits())
		if err != nil {
			t.Fatalf("SnapshotAt 失败: %v", err)
		}
		if g.EdgeCount() != 0 {
			t.Errorf("T3 关系应已解除，实际仍有 %d 条边", g.EdgeCount())
		}
	})

	t.Run("早于任何事件的时刻是空图", func(t *testing.T) {
		g, _, err := SnapshotAt(ctx, store, baseTime.Add(-time.Hour), graph.DefaultLimits())
		if err != nil {
			t.Fatalf("SnapshotAt 失败: %v", err)
		}
		if g.NodeCount() != 0 {
			t.Errorf("期望空图，实际 %d 个节点", g.NodeCount())
		}
	})
}

func TestDiffTopology(t *testing.T) {
	store := NewMemoryStore()
	t1, _, t3 := seedTimeline(t, store)
	ctx := t.Context()

	before, _, err := SnapshotAt(ctx, store, t1, graph.DefaultLimits())
	if err != nil {
		t.Fatalf("SnapshotAt(T1) 失败: %v", err)
	}
	after, _, err := SnapshotAt(ctx, store, t3, graph.DefaultLimits())
	if err != nil {
		t.Fatalf("SnapshotAt(T3) 失败: %v", err)
	}

	d := Diff(before.Snapshot(), after.Snapshot(), t1, t3)

	if d.Summary.EdgesRemoved != 1 {
		t.Errorf("期望 1 条关系被解除，实际 %d", d.Summary.EdgesRemoved)
	}
	if d.Summary.NodesModified != 1 {
		t.Fatalf("期望 1 个节点被修改，实际 %d", d.Summary.NodesModified)
	}
	if d.Summary.NodesAdded != 0 || d.Summary.NodesRemoved != 0 {
		t.Errorf("期望无节点增删，实际 +%d -%d", d.Summary.NodesAdded, d.Summary.NodesRemoved)
	}

	ch := d.ModifiedNodes[0]
	if ch.ID != "app" {
		t.Fatalf("期望修改的是 app，实际 %s", ch.ID)
	}
	found := false
	for _, c := range ch.Changes {
		if c.Field == "properties.risk_level" {
			found = true
			if c.Before != nil {
				t.Errorf("risk_level 变更前应为空，实际 %v", c.Before)
			}
			if c.After != "high" {
				t.Errorf("risk_level 变更后应为 high，实际 %v", c.After)
			}
		}
	}
	if !found {
		t.Errorf("差异中应包含 properties.risk_level，实际 %+v", ch.Changes)
	}
}

func TestDiffEmptySnapshots(t *testing.T) {
	d := Diff(nil, nil, baseTime, baseTime)
	if d.Summary.TotalDifference != 0 {
		t.Errorf("空快照对比应无差异，实际 %d", d.Summary.TotalDifference)
	}
	// 切片必须初始化为空数组而非 nil，否则 JSON 序列化成 null，前端得判空。
	if d.AddedNodes == nil || d.RemovedEdges == nil {
		t.Error("差异结果的切片字段应为空数组而非 nil")
	}
}

func TestStoreListFiltering(t *testing.T) {
	store := NewMemoryStore()
	seedTimeline(t, store)
	ctx := t.Context()

	t.Run("按实体过滤", func(t *testing.T) {
		got, total, err := store.List(ctx, ListFilter{EntityID: "app"})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 2 {
			t.Errorf("app 应有 2 条变更记录（创建+更新），实际 %d", total)
		}
		for _, e := range got {
			if e.EntityID != "app" {
				t.Errorf("过滤失效，出现了 %s 的事件", e.EntityID)
			}
		}
	})

	t.Run("按事件类型过滤", func(t *testing.T) {
		_, total, err := store.List(ctx, ListFilter{Types: []EventType{EventEdgeDeleted}})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 1 {
			t.Errorf("期望 1 条关系删除事件，实际 %d", total)
		}
	})

	t.Run("按时间范围过滤", func(t *testing.T) {
		to := baseTime.Add(30 * time.Minute)
		_, total, err := store.List(ctx, ListFilter{To: &to})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 3 {
			t.Errorf("T1 时刻应有 3 条事件，实际 %d", total)
		}
	})

	t.Run("倒序分页", func(t *testing.T) {
		got, total, err := store.List(ctx, ListFilter{Desc: true, Limit: 2})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 5 {
			t.Errorf("总数应为 5，实际 %d", total)
		}
		if len(got) != 2 {
			t.Fatalf("期望返回 2 条，实际 %d", len(got))
		}
		if got[0].Seq < got[1].Seq {
			t.Error("倒序模式下序列号应递减")
		}
	})

	t.Run("越界偏移返回空", func(t *testing.T) {
		got, _, err := store.List(ctx, ListFilter{Offset: 999})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("越界偏移应返回空，实际 %d 条", len(got))
		}
	})
}

func TestStoreAppendAssignsSeq(t *testing.T) {
	store := NewMemoryStore()
	n1 := testNode("a", "A", graph.NodeTypeServer, nil)
	n2 := testNode("b", "B", graph.NodeTypeServer, nil)

	e1, _ := NewNodeEvent(EventNodeCreated, n1, nil, "t", "", baseTime)
	e2, _ := NewNodeEvent(EventNodeCreated, n2, nil, "t", "", baseTime)
	if err := store.Append(t.Context(), []*Event{e1, e2}); err != nil {
		t.Fatalf("Append 失败: %v", err)
	}
	if e1.Seq == 0 || e2.Seq <= e1.Seq {
		t.Errorf("序列号应严格递增，实际 e1=%d e2=%d", e1.Seq, e2.Seq)
	}
	if e1.TypeLabel == "" {
		t.Error("应回填中文事件标签")
	}
}

func TestStoreAppendRejectsInvalid(t *testing.T) {
	store := NewMemoryStore()
	bad := &Event{Type: "bogus", EntityType: EntityNode, EntityID: "a"}
	if err := store.Append(t.Context(), []*Event{bad}); err == nil {
		t.Fatal("非法事件应被拒绝")
	}
	if n, _ := store.Count(t.Context()); n != 0 {
		t.Errorf("拒绝后不应留下任何记录，实际 %d 条", n)
	}
}

func assertSameTopology(t *testing.T, a, b *graph.Graph) {
	t.Helper()
	if a.NodeCount() != b.NodeCount() || a.EdgeCount() != b.EdgeCount() {
		t.Fatalf("拓扑规模不一致: %d/%d 节点，%d/%d 边",
			a.NodeCount(), b.NodeCount(), a.EdgeCount(), b.EdgeCount())
	}
	an, bn := a.Nodes(), b.Nodes()
	for i := range an {
		if an[i].ID != bn[i].ID || an[i].Name != bn[i].Name {
			t.Fatalf("第 %d 个节点不一致: %+v vs %+v", i, an[i], bn[i])
		}
	}
	ae, be := a.Edges(), b.Edges()
	for i := range ae {
		if ae[i].ID != be[i].ID || ae[i].Weight != be[i].Weight {
			t.Fatalf("第 %d 条边不一致: %+v vs %+v", i, ae[i], be[i])
		}
	}
}

var _ = context.Background

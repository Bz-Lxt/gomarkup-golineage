package graph

import (
	"errors"
	"fmt"
	"testing"
)

func TestAnalyzeSubgraphMatrix(t *testing.T) {
	g := mustGraph(t)

	res, err := g.AnalyzeSubgraphMatrix(nil)
	if err != nil {
		t.Fatalf("AnalyzeSubgraphMatrix 失败: %v", err)
	}

	if len(res.NodeIDs) != 7 {
		t.Fatalf("期望 7 行/列，实际 %d", len(res.NodeIDs))
	}
	if len(res.Matrix) != 7 || len(res.Matrix[0]) != 7 {
		t.Fatalf("矩阵维度应为 7x7，实际 %dx%d", len(res.Matrix), len(res.Matrix[0]))
	}
	if res.EdgeCount != 7 {
		t.Errorf("期望 7 条边，实际 %d", res.EdgeCount)
	}
	// 密度 = 7 / (7*6) ≈ 0.1667
	if res.Density < 0.16 || res.Density > 0.17 {
		t.Errorf("密度期望约 0.1667，实际 %v", res.Density)
	}
	// 测试图是连通的（弱连通意义上）。
	if res.ComponentCount != 1 {
		t.Errorf("期望 1 个连通分量，实际 %d: %v", res.ComponentCount, res.Components)
	}
	if res.LargestComponent != 7 {
		t.Errorf("最大连通分量期望 7，实际 %d", res.LargestComponent)
	}

	// 矩阵单元应准确反映权重。
	pos := make(map[NodeID]int)
	for i, id := range res.NodeIDs {
		pos[id] = i
	}
	if got := res.Matrix[pos["api-gw"]][pos["svc-order"]]; got != 3 {
		t.Errorf("api-gw→svc-order 权重应为 3，矩阵中为 %v", got)
	}
	if got := res.Matrix[pos["svc-order"]][pos["api-gw"]]; got != 0 {
		t.Errorf("有向边不应在反方向留下权重，实际 %v", got)
	}
}

// TestTwoHopReach 矩阵自乘给出恰好两跳的路径条数，
// 这是邻接矩阵相对邻接表的独有能力。
func TestTwoHopReach(t *testing.T) {
	g := mustGraph(t)
	res, err := g.AnalyzeSubgraphMatrix(nil)
	if err != nil {
		t.Fatalf("AnalyzeSubgraphMatrix 失败: %v", err)
	}

	pos := make(map[NodeID]int)
	for i, id := range res.NodeIDs {
		pos[id] = i
	}

	// app-web 两跳可达 svc-user：app-web→api-gw→svc-user，恰好 1 条。
	if got := res.TwoHopReach[pos["app-web"]][pos["svc-user"]]; got != 1 {
		t.Errorf("app-web 两跳到 svc-user 期望 1 条，实际 %d", got)
	}
	// app-web 两跳可达 db-user：走 e4 直连 svc-user 再到 db-user，恰好 1 条。
	// 注意经 api-gw 那条是 3 跳，不计入两跳矩阵。
	if got := res.TwoHopReach[pos["app-web"]][pos["db-user"]]; got != 1 {
		t.Errorf("app-web 两跳到 db-user 期望 1 条，实际 %d", got)
	}
	// db-order 最短也要 3 跳（app-web→api-gw→svc-order→db-order），两跳应为 0。
	if got := res.TwoHopReach[pos["app-web"]][pos["db-order"]]; got != 0 {
		t.Errorf("app-web 两跳不应到达 db-order，实际 %d 条", got)
	}
	// api-gw 两跳可达 db-user：api-gw→svc-user→db-user。
	if got := res.TwoHopReach[pos["api-gw"]][pos["db-user"]]; got != 1 {
		t.Errorf("api-gw 两跳到 db-user 期望 1 条，实际 %d", got)
	}
}

func TestMatrixDisconnectedComponents(t *testing.T) {
	g := New(DefaultLimits())
	// 两个互不相连的小团 + 一个孤立点。
	for _, id := range []string{"a1", "a2", "b1", "b2", "lonely"} {
		if err := g.AddNode(newNode(id, "节点"+id, NodeTypeApplication, nil)); err != nil {
			t.Fatal(err)
		}
	}
	_ = g.AddEdge(newEdge("ea", "a1", "a2", RelCalls, 1))
	_ = g.AddEdge(newEdge("eb", "b1", "b2", RelCalls, 1))

	res, err := g.AnalyzeSubgraphMatrix(nil)
	if err != nil {
		t.Fatalf("AnalyzeSubgraphMatrix 失败: %v", err)
	}
	if res.ComponentCount != 3 {
		t.Fatalf("期望 3 个连通分量，实际 %d: %v", res.ComponentCount, res.Components)
	}
	if res.LargestComponent != 2 {
		t.Errorf("最大分量期望 2，实际 %d", res.LargestComponent)
	}
	// 分量按规模降序，孤立点排在最后。
	last := res.Components[len(res.Components)-1]
	if len(last) != 1 || last[0] != "lonely" {
		t.Errorf("最后一个分量应为孤立点 lonely，实际 %v", last)
	}
}

func TestMatrixSubsetSelection(t *testing.T) {
	g := mustGraph(t)
	res, err := g.AnalyzeSubgraphMatrix([]NodeID{"app-web", "api-gw", "svc-user"})
	if err != nil {
		t.Fatalf("AnalyzeSubgraphMatrix 失败: %v", err)
	}
	if len(res.NodeIDs) != 3 {
		t.Fatalf("期望 3 个节点，实际 %d", len(res.NodeIDs))
	}
	// 子集内部有 e1、e2、e4 三条边。
	if res.EdgeCount != 3 {
		t.Errorf("期望 3 条内部边，实际 %d", res.EdgeCount)
	}
}

func TestMatrixRejectsOversizedSubgraph(t *testing.T) {
	g := New(DefaultLimits())
	for i := 0; i <= MaxMatrixNodes; i++ {
		id := fmt.Sprintf("n%04d", i)
		if err := g.AddNode(newNode(id, id, NodeTypeStorage, nil)); err != nil {
			t.Fatal(err)
		}
	}

	// 超限时必须报错而非静默截断 —— 截断后的密度与连通分量都是错误结论。
	_, err := g.AnalyzeSubgraphMatrix(nil)
	if !errors.Is(err, ErrSubgraphTooLarge) {
		t.Fatalf("期望 ErrSubgraphTooLarge，实际: %v", err)
	}
}

func TestMatrixUnknownNode(t *testing.T) {
	g := mustGraph(t)
	if _, err := g.AnalyzeSubgraphMatrix([]NodeID{"app-web", "ghost"}); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("期望 ErrNodeNotFound，实际: %v", err)
	}
}

func TestMatrixEmptyGraph(t *testing.T) {
	g := New(DefaultLimits())
	res, err := g.AnalyzeSubgraphMatrix(nil)
	if err != nil {
		t.Fatalf("空图分析不应报错: %v", err)
	}
	if len(res.NodeIDs) != 0 || res.ComponentCount != 0 {
		t.Errorf("空图应返回空结果，实际 %+v", res)
	}
}

func TestMatrixUndirectedSymmetry(t *testing.T) {
	g := New(DefaultLimits())
	_ = g.AddNode(newNode("p1", "张三", NodeTypePerson, nil))
	_ = g.AddNode(newNode("p2", "李四", NodeTypePerson, nil))
	_ = g.AddEdge(&Edge{ID: "u1", Source: "p1", Target: "p2", Relation: RelAssociatesWith, Weight: 2, Directed: false})

	res, err := g.AnalyzeSubgraphMatrix(nil)
	if err != nil {
		t.Fatalf("AnalyzeSubgraphMatrix 失败: %v", err)
	}
	if res.Matrix[0][1] != 2 || res.Matrix[1][0] != 2 {
		t.Errorf("无向边应在矩阵中对称出现，实际 [0][1]=%v [1][0]=%v", res.Matrix[0][1], res.Matrix[1][0])
	}
}

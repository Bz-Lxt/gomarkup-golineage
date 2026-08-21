package api

import (
	"net/http"

	"github.com/alkaid/golineage/internal/service"
)

// getTopology GET /api/v1/graph —— 全量拓扑（默认裁剪为核心子图）。
//
// 默认 limit 为 500：前端画布渲染上万节点必然卡死，
// 因此默认只下发度数最高的核心骨架，其余由用户按需展开邻居。
func (h *Handler) getTopology(w http.ResponseWriter, r *http.Request) {
	topo, err := h.svc.Topology(r.Context(), service.TopologyQuery{
		Types:     queryList(r, "type"),
		Relations: queryList(r, "relation"),
		Limit:     queryInt(r, "limit", 500),
	})
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, topo)
}

// getNeighbors GET /api/v1/graph/neighbors —— 邻居子图，前端点击高亮的数据源。
func (h *Handler) getNeighbors(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Neighbors(r.Context(),
		queryString(r, "id"),
		queryInt(r, "hops", 1),
		queryList(r, "relation"),
	)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// traverse GET /api/v1/graph/traverse —— BFS / DFS 遍历。
func (h *Handler) traverse(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Traverse(r.Context(), service.TraverseQuery{
		Algorithm: queryString(r, "algorithm"),
		Start:     queryString(r, "start"),
		MaxDepth:  queryInt(r, "max_depth", 0),
		Direction: queryString(r, "direction"),
		Relations: queryList(r, "relation"),
		NodeTypes: queryList(r, "type"),
		MaxNodes:  queryInt(r, "max_nodes", 0),
	})
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// shortestPath GET /api/v1/graph/shortest-path —— Dijkstra 最短路径。
func (h *Handler) shortestPath(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.ShortestPath(r.Context(), service.PathQuery{
		From:      queryString(r, "from"),
		To:        queryString(r, "to"),
		Direction: queryString(r, "direction"),
		Relations: queryList(r, "relation"),
	})
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// allPaths GET /api/v1/graph/all-paths —— 全路径枚举。
func (h *Handler) allPaths(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.AllPaths(r.Context(), service.PathQuery{
		From:      queryString(r, "from"),
		To:        queryString(r, "to"),
		Direction: queryString(r, "direction"),
		Relations: queryList(r, "relation"),
		MaxDepth:  queryInt(r, "max_depth", 0),
		MaxPaths:  queryInt(r, "max_paths", 0),
	})
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// lineage GET /api/v1/graph/lineage —— 上下游血缘分析。
func (h *Handler) lineage(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Lineage(r.Context(), service.LineageQuery{
		Root:      queryString(r, "root"),
		MaxDepth:  queryInt(r, "max_depth", 0),
		Relations: queryList(r, "relation"),
		MaxNodes:  queryInt(r, "max_nodes", 0),
	})
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// matrixBody 邻接矩阵分析的请求体。
type matrixBody struct {
	NodeIDs []string `json:"node_ids"`
}

// matrixAnalysis POST /api/v1/graph/matrix —— 子图邻接矩阵结构分析。
//
// 用 POST 而非 GET：节点 ID 列表可能有数百个，
// 放在查询串里会超出部分代理的 URL 长度限制。
func (h *Handler) matrixAnalysis(w http.ResponseWriter, r *http.Request) {
	var body matrixBody
	if r.ContentLength > 0 && !decodeJSON(w, r, &body) {
		return
	}

	res, err := h.svc.MatrixAnalysis(r.Context(), body.NodeIDs)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// structure GET /api/v1/graph/structure —— 结构概览与循环依赖检测。
func (h *Handler) structure(w http.ResponseWriter, r *http.Request) {
	OK(w, r, h.svc.Structure(r.Context()))
}

// metadata GET /api/v1/meta —— 前端初始化所需的枚举与限制。
func (h *Handler) metadata(w http.ResponseWriter, r *http.Request) {
	OK(w, r, h.svc.Metadata(r.Context()))
}

package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alkaid/golineage/internal/service"
)

// listNodes GET /api/v1/nodes —— 检索资产节点。
func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.svc.SearchNodes(r.Context(), service.SearchQuery{
		Keyword:    queryString(r, "keyword"),
		NamePrefix: queryString(r, "name_prefix"),
		Types:      queryList(r, "type"),
		PropKey:    queryString(r, "prop_key"),
		PropValue:  queryString(r, "prop_value"),
		Limit:      queryInt(r, "limit", 200),
	})
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, map[string]any{"items": nodes, "count": len(nodes)})
}

// getNode GET /api/v1/nodes/{id} —— 读取资产详情。
func (h *Handler) getNode(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.NodeDetail(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, detail)
}

// createNode POST /api/v1/nodes —— 新建资产。
func (h *Handler) createNode(w http.ResponseWriter, r *http.Request) {
	var in service.CreateNodeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Actor = actorFrom(r, in.Actor)

	n, err := h.svc.CreateNode(r.Context(), in)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	Created(w, r, n)
}

// updateNode PUT /api/v1/nodes/{id} —— 修改资产，含动态属性增删改。
func (h *Handler) updateNode(w http.ResponseWriter, r *http.Request) {
	var in service.UpdateNodeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Actor = actorFrom(r, in.Actor)

	n, err := h.svc.UpdateNode(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, n)
}

// deleteNodeBody 删除资产的可选请求体。
type deleteNodeBody struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// deleteNode DELETE /api/v1/nodes/{id} —— 删除资产并级联解除关系。
func (h *Handler) deleteNode(w http.ResponseWriter, r *http.Request) {
	var body deleteNodeBody
	// 删除请求允许没有请求体，此时原因留空即可，不应因此报错。
	if r.ContentLength > 0 && !decodeJSON(w, r, &body) {
		return
	}

	res, err := h.svc.DeleteNode(r.Context(), chi.URLParam(r, "id"),
		actorFrom(r, body.Actor), body.Reason)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// nodeImpact GET /api/v1/nodes/{id}/impact —— 影响面评估，删除前的风险提示。
//
// 深度参数取名 max_depth，与其余图查询端点（traverse / lineage / all-paths）保持一致：
// 前端统一按 max_depth 编码查询串，此处若改用 depth 会读不到值而回退到 0，
// 进而被 BFS 归一化为全局上限，导致用户设置的追溯深度限制失效。
func (h *Handler) nodeImpact(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Impact(r.Context(), chi.URLParam(r, "id"), queryInt(r, "max_depth", 0))
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, summary)
}

// nodeEvents GET /api/v1/nodes/{id}/events —— 单个资产的变更历史时间线。
func (h *Handler) nodeEvents(w http.ResponseWriter, r *http.Request) {
	page, err := h.svc.EntityTimeline(r.Context(), chi.URLParam(r, "id"), queryInt(r, "limit", 50))
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, page)
}

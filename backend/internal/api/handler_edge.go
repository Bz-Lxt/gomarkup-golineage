package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/alkaid/golineage/internal/service"
)

// getEdge GET /api/v1/edges/{id} —— 读取血缘关系。
func (h *Handler) getEdge(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.GetEdge(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, e)
}

// createEdge POST /api/v1/edges —— 建立血缘关系。
func (h *Handler) createEdge(w http.ResponseWriter, r *http.Request) {
	var in service.CreateEdgeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Actor = actorFrom(r, in.Actor)

	e, err := h.svc.CreateEdge(r.Context(), in)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	Created(w, r, e)
}

// updateEdge PUT /api/v1/edges/{id} —— 修改关系的类型、权重与属性。
func (h *Handler) updateEdge(w http.ResponseWriter, r *http.Request) {
	var in service.UpdateEdgeInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Actor = actorFrom(r, in.Actor)

	e, err := h.svc.UpdateEdge(r.Context(), chi.URLParam(r, "id"), in)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, e)
}

// deleteEdgeBody 解除关系的可选请求体。
type deleteEdgeBody struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// deleteEdge DELETE /api/v1/edges/{id} —— 解除血缘关系。
//
// 这是「A 应用不再调用 B 数据库」这类变更的入口，
// 建议携带 reason，它会成为变更流水中最有价值的上下文。
func (h *Handler) deleteEdge(w http.ResponseWriter, r *http.Request) {
	var body deleteEdgeBody
	if r.ContentLength > 0 && !decodeJSON(w, r, &body) {
		return
	}

	e, err := h.svc.DeleteEdge(r.Context(), chi.URLParam(r, "id"),
		actorFrom(r, body.Actor), body.Reason)
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, e)
}

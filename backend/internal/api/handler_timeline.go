package api

import (
	"net/http"

	"github.com/alkaid/golineage/internal/service"
)

// listEvents GET /api/v1/timeline/events —— 血缘变更流水。
func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	page, err := h.svc.ListEvents(r.Context(), service.EventQuery{
		EntityID: queryString(r, "entity_id"),
		Types:    queryList(r, "event_type"),
		From:     queryString(r, "from"),
		To:       queryString(r, "to"),
		Actor:    queryString(r, "actor"),
		Limit:    queryInt(r, "limit", 50),
		Offset:   queryInt(r, "offset", 0),
		Desc:     queryBool(r, "desc", false),
	})
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, page)
}

// timelineSnapshot GET /api/v1/timeline/snapshot —— 重建指定时刻的历史拓扑。
//
// 这是时间轴回溯的核心接口：拖动滑块时前端携带时间戳请求，
// 后端在独立图实例上重放事件，不影响任何人的实时视图。
func (h *Handler) timelineSnapshot(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.SnapshotAt(r.Context(), queryString(r, "at"), queryInt(r, "limit", 500))
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// timelineDiff GET /api/v1/timeline/diff —— 两个时刻之间的拓扑差异。
func (h *Handler) timelineDiff(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.DiffTopology(r.Context(), queryString(r, "from"), queryString(r, "to"))
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// timelineBounds GET /api/v1/timeline/bounds —— 时间轴的可用范围。
func (h *Handler) timelineBounds(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.TimelineBounds(r.Context())
	if err != nil {
		FailErr(w, r, err)
		return
	}
	OK(w, r, res)
}

// health GET /healthz —— 健康检查。
//
// 数据库不可用时返回 503，让容器编排能够真实感知服务状态，
// 而不是拿到一个永远 200 的假信号。
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	status := h.svc.Health(r.Context())
	if status.Status != "ok" {
		writeJSON(w, r, http.StatusServiceUnavailable, Response{
			Code: CodeStorage, Message: "服务降级", Data: status,
		})
		return
	}
	OK(w, r, status)
}

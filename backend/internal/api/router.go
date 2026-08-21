package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/alkaid/golineage/internal/service"
)

// Handler 聚合全部 HTTP 处理器所需的依赖。
type Handler struct {
	svc *service.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

// Router 装配全部路由与中间件。
func (h *Handler) Router(corsOrigins []string, requestTimeout time.Duration) http.Handler {
	r := chi.NewRouter()

	// 中间件顺序有讲究：TraceID 必须最先，后续日志才能携带链路 ID；
	// Recover 紧随其后，才能兜住内层所有中间件与处理器的 panic。
	r.Use(TraceID)
	r.Use(Recover)
	r.Use(AccessLog)
	r.Use(SecurityHeaders)
	r.Use(CORS(corsOrigins))
	r.Use(Timeout(requestTimeout))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		Fail(w, r, http.StatusNotFound, CodeRouteMissing, "接口不存在："+r.URL.Path)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		Fail(w, r, http.StatusMethodNotAllowed, CodeRouteMissing, "请求方法不被支持："+r.Method)
	})

	r.Get("/healthz", h.health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/meta", h.metadata)

		r.Route("/nodes", func(r chi.Router) {
			r.Get("/", h.listNodes)
			r.Post("/", h.createNode)
			r.Get("/{id}", h.getNode)
			r.Put("/{id}", h.updateNode)
			r.Delete("/{id}", h.deleteNode)
			r.Get("/{id}/impact", h.nodeImpact)
			r.Get("/{id}/events", h.nodeEvents)
		})

		r.Route("/edges", func(r chi.Router) {
			r.Post("/", h.createEdge)
			r.Get("/{id}", h.getEdge)
			r.Put("/{id}", h.updateEdge)
			r.Delete("/{id}", h.deleteEdge)
		})

		r.Route("/graph", func(r chi.Router) {
			r.Get("/", h.getTopology)
			r.Get("/neighbors", h.getNeighbors)
			r.Get("/traverse", h.traverse)
			r.Get("/shortest-path", h.shortestPath)
			r.Get("/all-paths", h.allPaths)
			r.Get("/lineage", h.lineage)
			r.Get("/structure", h.structure)
			r.Post("/matrix", h.matrixAnalysis)
		})

		r.Route("/timeline", func(r chi.Router) {
			r.Get("/events", h.listEvents)
			r.Get("/snapshot", h.timelineSnapshot)
			r.Get("/diff", h.timelineDiff)
			r.Get("/bounds", h.timelineBounds)
		})
	})

	return r
}

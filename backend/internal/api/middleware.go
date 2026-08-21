package api

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alkaid/golineage/pkg/logger"
)

// traceIDHeader 链路 ID 的响应头名称，便于用户在报障时直接提供。
const traceIDHeader = "X-Trace-Id"

// TraceID 为每个请求分配链路 ID 并注入 context 与响应头。
func TraceID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(traceIDHeader))
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(traceIDHeader, id)
		next.ServeHTTP(w, r.WithContext(logger.WithTraceID(r.Context(), id)))
	})
}

// statusRecorder 捕获响应状态码与字节数，供访问日志使用。
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// AccessLog 记录访问日志。
//
// 健康检查按 debug 级别记录：容器编排每 10 秒探测一次，
// 按 info 记录会让真正有价值的业务日志被淹没。
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		args := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"cost_ms", time.Since(start).Milliseconds(),
		}
		switch {
		case r.URL.Path == "/healthz":
			logger.DebugCtx(r.Context(), "请求完成", args...)
		case rec.status >= 500:
			logger.ErrorCtx(r.Context(), "请求失败", args...)
		case rec.status >= 400:
			logger.WarnCtx(r.Context(), "请求异常", args...)
		default:
			logger.InfoCtx(r.Context(), "请求完成", args...)
		}
	})
}

// Recover 捕获 panic，保证单个请求的崩溃不会拖垮整个进程。
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.ErrorCtx(r.Context(), "请求处理发生 panic",
					"path", r.URL.Path, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
				Fail(w, r, http.StatusInternalServerError, CodeInternal,
					"服务内部错误，请携带 trace_id 联系管理员")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS 按白名单放行跨域请求。
//
// 不使用通配符 *：生产环境下通配符会让任意站点都能读取本服务的数据。
// 前端经 Nginx 同源反代访问时本中间件不会命中，白名单只为直连调试保留。
func CORS(allowed []string) func(http.Handler) http.Handler {
	allowSet := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if o = strings.TrimSpace(o); o != "" {
			allowSet[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := allowSet[origin]; ok && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+traceIDHeader)
				w.Header().Set("Access-Control-Expose-Headers", traceIDHeader)
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders 附加基础安全响应头。
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// Timeout 为请求设置整体处理超时。
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, `{"code":40801,"message":"请求处理超时"}`)
	}
}

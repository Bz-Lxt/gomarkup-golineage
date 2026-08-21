// Package api 提供 HTTP 传输层：路由、中间件、请求解析与响应封装。
//
// 本层只负责协议转换与入参校验，不含任何业务规则；
// 业务规则一律下沉到 service 层，避免同一条规则在多个 handler 中重复实现。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alkaid/golineage/internal/graph"
	"github.com/alkaid/golineage/pkg/logger"
)

// 业务错误码。
//
// 与 HTTP 状态码分离：HTTP 状态码表达传输层语义，业务码表达具体失败原因，
// 便于前端做精细化提示而不必解析错误文案。
const (
	CodeOK = 0

	CodeBadRequest    = 40000 // 请求体解析失败
	CodeValidation    = 40001 // 参数校验失败
	CodeLimitExceeded = 40002 // 查询规模超过上限
	CodeSubgraphLarge = 40003 // 子图规模超过矩阵分析上限

	CodeNodeNotFound = 40401 // 节点不存在
	CodeEdgeNotFound = 40402 // 关系不存在
	CodeRouteMissing = 40404 // 接口不存在

	CodeNodeExists = 40901 // 节点已存在
	CodeEdgeExists = 40902 // 关系已存在

	CodeTimeout  = 40801 // 查询超时
	CodeInternal = 50000 // 服务内部错误
	CodeStorage  = 50001 // 存储不可用
)

// Response 统一响应包络。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, r *http.Request, status int, body Response) {
	body.TraceID = logger.TraceIDFrom(r.Context())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// 响应已开始写出，此时无法再改状态码，只能记录以便排查。
		logger.ErrorCtx(r.Context(), "序列化响应失败", "err", err)
	}
}

// OK 返回成功响应。
func OK(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, r, http.StatusOK, Response{Code: CodeOK, Message: "ok", Data: data})
}

// Created 返回 201 成功响应。
func Created(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, r, http.StatusCreated, Response{Code: CodeOK, Message: "created", Data: data})
}

// Fail 按指定状态码与业务码返回失败响应。
func Fail(w http.ResponseWriter, r *http.Request, status, code int, msg string) {
	writeJSON(w, r, status, Response{Code: code, Message: msg})
}

// FailErr 将 service / graph 层的错误映射为恰当的 HTTP 状态与业务码。
//
// 集中映射的意义在于：新增一种领域错误时只需在此处补一条分支，
// 而不必在每个 handler 里重复判断，也就不会出现「同一种错误在不同接口
// 返回不同状态码」的不一致。
func FailErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, graph.ErrValidation):
		Fail(w, r, http.StatusBadRequest, CodeValidation, err.Error())

	case errors.Is(err, graph.ErrNodeNotFound):
		Fail(w, r, http.StatusNotFound, CodeNodeNotFound, err.Error())

	case errors.Is(err, graph.ErrEdgeNotFound):
		Fail(w, r, http.StatusNotFound, CodeEdgeNotFound, err.Error())

	case errors.Is(err, graph.ErrNodeExists):
		Fail(w, r, http.StatusConflict, CodeNodeExists, err.Error())

	case errors.Is(err, graph.ErrEdgeExists):
		Fail(w, r, http.StatusConflict, CodeEdgeExists, err.Error())

	case errors.Is(err, graph.ErrLimitExceeded):
		Fail(w, r, http.StatusBadRequest, CodeLimitExceeded, err.Error())

	case errors.Is(err, graph.ErrSubgraphTooLarge):
		Fail(w, r, http.StatusBadRequest, CodeSubgraphLarge, err.Error())

	case errors.Is(err, context.DeadlineExceeded):
		logger.WarnCtx(r.Context(), "图查询超时", "path", r.URL.Path)
		Fail(w, r, http.StatusRequestTimeout, CodeTimeout,
			"查询超时，请缩小查询范围（降低深度或增加过滤条件）")

	case errors.Is(err, context.Canceled):
		// 客户端主动断开，无需按错误处理，也不必回写响应体。
		logger.DebugCtx(r.Context(), "客户端取消请求", "path", r.URL.Path)
		Fail(w, r, 499, CodeTimeout, "请求已被取消")

	default:
		logger.ErrorCtx(r.Context(), "服务内部错误", "path", r.URL.Path, "err", err)
		Fail(w, r, http.StatusInternalServerError, CodeInternal, "服务内部错误："+err.Error())
	}
}

// decodeJSON 解析请求体，并对空体与超大体给出明确提示。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	// 限制请求体大小，避免恶意超大 payload 耗尽内存。
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			Fail(w, r, http.StatusRequestEntityTooLarge, CodeBadRequest, "请求体过大（上限 1MB）")
			return false
		}
		Fail(w, r, http.StatusBadRequest, CodeBadRequest, "请求体解析失败："+err.Error())
		return false
	}
	return true
}

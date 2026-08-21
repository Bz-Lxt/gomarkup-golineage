package graph

import "errors"

// 图引擎的哨兵错误。
//
// 上层通过 errors.Is 判定错误类别并映射为对应的 HTTP 状态码，
// 避免把内部错误细节直接透传给调用方。
var (
	// ErrValidation 入参校验失败，映射 400。
	ErrValidation = errors.New("参数校验失败")

	// ErrNodeNotFound 节点不存在，映射 404。
	ErrNodeNotFound = errors.New("节点不存在")

	// ErrEdgeNotFound 边不存在，映射 404。
	ErrEdgeNotFound = errors.New("关系不存在")

	// ErrNodeExists 节点 ID 重复，映射 409。
	ErrNodeExists = errors.New("节点已存在")

	// ErrEdgeExists 同起点、终点、关系类型的边重复，映射 409。
	ErrEdgeExists = errors.New("关系已存在")

	// ErrLimitExceeded 查询规模超过安全上限，映射 400。
	ErrLimitExceeded = errors.New("查询规模超过上限")

	// ErrSubgraphTooLarge 子图规模超过邻接矩阵分析上限，映射 400。
	ErrSubgraphTooLarge = errors.New("子图规模超过邻接矩阵分析上限")
)

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alkaid/golineage/internal/graph"
)

func TestMatrixAnalysisAcceptsNullBody(t *testing.T) {
	h := newTestServer(t)
	createNode(t, h, "订单服务", "application", nil)
	createNode(t, h, "订单库", "database", nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/graph/matrix", strings.NewReader("null"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("空选择应分析全图，期望 200，实际 %d: %s", rec.Code, rec.Body.String())
	}

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v，原文: %s", err, rec.Body.String())
	}
	var analysis graph.MatrixAnalysis
	dataAs(t, resp, &analysis)
	if len(analysis.NodeIDs) != 2 {
		t.Fatalf("空选择应包含全图 2 个资产，实际 %d: %v", len(analysis.NodeIDs), analysis.NodeIDs)
	}
}

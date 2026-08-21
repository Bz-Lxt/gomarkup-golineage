package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alkaid/golineage/internal/service"
)

func TestExplicitActorTakesPrecedenceOverHeader(t *testing.T) {
	h := newTestServer(t)
	body := []byte(`{"name":"代维护资产","type":"application","actor":"on-call-engineer"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor", "gateway-user")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("创建资产期望 201，实际 %d: %s", rec.Code, rec.Body.String())
	}

	status, resp := call(t, h, http.MethodGet, "/api/v1/timeline/events?limit=1", nil)
	if status != http.StatusOK {
		t.Fatalf("查询变更流水期望 200，实际 %d: %+v", status, resp)
	}
	var page service.EventPage
	dataAs(t, resp, &page)
	if len(page.Items) != 1 {
		t.Fatalf("期望返回 1 条变更流水，实际 %d", len(page.Items))
	}
	if got := page.Items[0].Actor; got != "on-call-engineer" {
		encoded, _ := json.Marshal(page.Items[0])
		t.Fatalf("显式 actor 应写入变更流水，期望 on-call-engineer，实际 %q；事件=%s", got, encoded)
	}
}

package api

import (
	"net/http"
	"testing"

	"github.com/alkaid/golineage/internal/eventstore"
	"github.com/alkaid/golineage/internal/service"
)

func TestTimelineEventsDefaultNewestFirst(t *testing.T) {
	h := newTestServer(t)
	id := createNode(t, h, "订单服务", "application", nil)

	status, resp := call(t, h, http.MethodPut, "/api/v1/nodes/"+id, map[string]any{
		"name":   "订单服务 v2",
		"reason": "服务升级",
	})
	if status != http.StatusOK {
		t.Fatalf("更新节点失败: status=%d resp=%+v", status, resp)
	}

	status, resp = call(t, h, http.MethodGet, "/api/v1/timeline/events?limit=10", nil)
	if status != http.StatusOK {
		t.Fatalf("查询变更流水失败: status=%d resp=%+v", status, resp)
	}

	var page service.EventPage
	dataAs(t, resp, &page)
	if len(page.Items) != 2 {
		t.Fatalf("期望返回创建和更新两条流水，实际 %d 条", len(page.Items))
	}
	if page.Items[0].Type != eventstore.EventNodeUpdated {
		t.Fatalf("未指定排序时应将最新变更放在首位，实际首项类型为 %q", page.Items[0].Type)
	}
	if page.Items[0].Seq <= page.Items[1].Seq {
		t.Fatalf("未指定排序时流水应按序列号倒序，实际 seq=%d,%d", page.Items[0].Seq, page.Items[1].Seq)
	}
}

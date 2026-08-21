package api

import (
	"net/http"
	"strconv"
	"strings"
)

// queryString 读取字符串查询参数并去除首尾空白。
func queryString(r *http.Request, key string) string {
	return strings.TrimSpace(r.URL.Query().Get(key))
}

// queryInt 读取整型查询参数，缺失或非法时返回默认值。
func queryInt(r *http.Request, key string, def int) int {
	raw := queryString(r, key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

// queryBool 读取布尔查询参数。
func queryBool(r *http.Request, key string, def bool) bool {
	raw := queryString(r, key)
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return b
}

// queryList 读取多值查询参数。
//
// 同时支持 ?types=a&types=b 与 ?types=a,b 两种写法：
// 前者是 HTTP 惯例，后者在手写 URL 与文档示例中更易读。
func queryList(r *http.Request, key string) []string {
	values := []string{r.URL.Query().Get(key)}
	out := make([]string, 0, len(values))
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// actorFrom 解析操作者标识。
//
// 优先取请求体中的 actor，其次取请求头 X-Actor。
// 本项目按需求约定不做用户体系，此处仅用于变更流水的归属标注。
func actorFrom(r *http.Request, bodyActor string) string {
	if a := strings.TrimSpace(bodyActor); a != "" {
		return a
	}
	if a := strings.TrimSpace(r.Header.Get("X-Actor")); a != "" {
		return a
	}
	return "anonymous"
}

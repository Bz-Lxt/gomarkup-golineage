// Package timeutil 统一项目内的时区处理。
//
// 工作区规范要求所有时间以 GMT+8 北京时区为准。
// 直接使用 time.Now() 在容器 TZ 未生效时会退化为 UTC，导致展示时间相差 8 小时，
// 因此项目内一律通过本包获取时间。
package timeutil

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Beijing 北京时区（GMT+8）。使用 FixedZone 而非 LoadLocation，
// 避免依赖容器内是否安装 tzdata。
var Beijing = time.FixedZone("CST", 8*60*60)

// Now 返回当前北京时间。
func Now() time.Time { return time.Now().In(Beijing) }

// To 将任意时间转换为北京时区表示（时刻不变，仅改变展示时区）。
func To(t time.Time) time.Time { return t.In(Beijing) }

// Format 以 "2006-01-02 15:04:05" 格式化为北京时间字符串。
func Format(t time.Time) string { return To(t).Format("2006-01-02 15:04:05") }

// FormatRFC3339 以 RFC3339 格式化为带 +08:00 偏移的字符串。
func FormatRFC3339(t time.Time) string { return To(t).Format(time.RFC3339) }

// mangledOffset 匹配时区偏移中的加号被解码成空格后的形态，例如
// "2026-08-21T20:51:10 08:00"。
//
// 这是 HTTP 查询参数的经典陷阱：RFC3339 的 "+08:00" 若未经 URL 编码，
// 服务端解析时 "+" 会被还原为空格，时间字符串随即失效。
// 与其要求每个调用方都记得编码，不如在此处兼容。
var mangledOffset = regexp.MustCompile(`T\d{2}:\d{2}:\d{2}(\.\d+)? (\d{2}:\d{2})$`)

// Parse 解析时间字符串，依次尝试 RFC3339（含纳秒）、
// "2006-01-02 15:04:05" 与 "2006-01-02" 等格式。
// 不含时区信息的格式按北京时间解释，避免被当作 UTC 而产生 8 小时偏差。
func Parse(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := parseKnownLayouts(s); err == nil {
		return t, nil
	}

	if loc := mangledOffset.FindStringIndex(s); loc != nil {
		fixed := s[:len(s)-6] + "+" + s[len(s)-5:]
		if t, err := parseKnownLayouts(fixed); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q", s)
}

func parseKnownLayouts(s string) (time.Time, error) {
	for _, l := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(l, s); err == nil {
			return t.In(Beijing), nil
		}
	}
	// 不含时区的格式一律按北京时间解释。
	for _, l := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(l, s, Beijing); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q", s)
}

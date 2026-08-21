// Package logger 提供全局统一的分级日志能力。
//
// 项目内禁止使用 fmt.Println / log.Println 直接输出，
// 所有日志必须经由本包，以保证级别可控、格式统一、便于问题定位。
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Level 日志级别。
type Level = slog.Level

const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// traceIDKey 用于从 context 中提取请求链路 ID。
type traceIDKey struct{}

// WithTraceID 将链路 ID 注入 context，后续日志会自动携带。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFrom 从 context 提取链路 ID，不存在时返回空串。
func TraceIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

var (
	mu      sync.RWMutex
	current *slog.Logger
	lvlVar  = new(slog.LevelVar)
)

func init() {
	lvlVar.Set(LevelInfo)
	current = slog.New(newHandler(os.Stdout, lvlVar))
}

// ParseLevel 将字符串解析为日志级别，无法识别时回退到 info。
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Init 按指定级别初始化全局 Logger。生产环境传 info 即可自动屏蔽 debug 输出。
func Init(level Level, w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	if w == nil {
		w = os.Stdout
	}
	lvlVar.Set(level)
	current = slog.New(newHandler(w, lvlVar))
}

// SetLevel 运行时动态调整日志级别。
func SetLevel(level Level) { lvlVar.Set(level) }

// Enabled 报告指定级别当前是否会被输出，可用于跳过昂贵的日志参数构造。
func Enabled(level Level) bool { return lvlVar.Level() <= level }

// L 返回全局 Logger。
func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// With 返回带固定字段的子 Logger。
func With(args ...any) *slog.Logger { return L().With(args...) }

func Debug(msg string, args ...any) { L().Debug(msg, args...) }
func Info(msg string, args ...any)  { L().Info(msg, args...) }
func Warn(msg string, args ...any)  { L().Warn(msg, args...) }
func Error(msg string, args ...any) { L().Error(msg, args...) }

// DebugCtx 等带 Ctx 后缀的方法会自动附加 context 中的 trace_id。
func DebugCtx(ctx context.Context, msg string, args ...any) {
	L().Debug(msg, withTrace(ctx, args)...)
}

func InfoCtx(ctx context.Context, msg string, args ...any) {
	L().Info(msg, withTrace(ctx, args)...)
}

func WarnCtx(ctx context.Context, msg string, args ...any) {
	L().Warn(msg, withTrace(ctx, args)...)
}

func ErrorCtx(ctx context.Context, msg string, args ...any) {
	L().Error(msg, withTrace(ctx, args)...)
}

func withTrace(ctx context.Context, args []any) []any {
	if id := TraceIDFrom(ctx); id != "" {
		return append([]any{"trace_id", id}, args...)
	}
	return args
}

// beijing 固定为 GMT+8，保证容器时区未生效时日志时间仍然正确。
var beijing = time.FixedZone("CST", 8*60*60)

// newHandler 构造带 GMT+8 时间戳的文本处理器。
func newHandler(w io.Writer, lv *slog.LevelVar) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.In(beijing).Format("2006-01-02 15:04:05.000"))
				}
			}
			return a
		},
	})
}

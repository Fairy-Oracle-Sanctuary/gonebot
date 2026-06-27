// Package logger 提供人类可读的结构化日志.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"neobot/core/internal/logx"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 默认 logger.
var (
	defaultLogger *slog.Logger
	once          sync.Once
)

// Setup 初始化全局 logger.
//
//	level:  debug|info|warn|error
//	output: stdout|file|both
//	file:   日志文件路径 (output 包含 file 时生效)
func Setup(level, output, file string) error {
	var err error
	once.Do(func() {
		err = setup(level, output, file)
	})
	return err
}

func setup(level, output, file string) error {
	lvl := parseLevel(level)

	// 控制台: 彩色文本
	stdoutHandler := newPrettyHandler(os.Stdout, true, lvl)
	// 文件: 纯文本 (无颜色, 便于日志采集)
	fileHandler := newPrettyHandler(nil, false, lvl)

	var w io.Writer
	_ = w
	switch output {
	case "file":
		if file == "" {
			return fmt.Errorf("output=file requires file path")
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		fileHandler.w = f
		defaultLogger = slog.New(logx.WrapHandler(fileHandler))
	case "both":
		if file != "" {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err == nil {
				if f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
					fileHandler.w = f
				}
			}
		}
		defaultLogger = slog.New(logx.WrapHandler(multiHandler{stdoutHandler, fileHandler}))
	default: // stdout
		defaultLogger = slog.New(logx.WrapHandler(stdoutHandler))
	}
	slog.SetDefault(defaultLogger)
	return nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// L 返回默认 logger.
func L() *slog.Logger {
	if defaultLogger == nil {
		defaultLogger = slog.New(newPrettyHandler(os.Stdout, false, slog.LevelInfo))
	}
	return defaultLogger
}

// Module 返回带 module 字段的子 logger.
func Module(name string) *slog.Logger {
	return L().With("module", name)
}

// WithCtx 返回带 context 的 logger.
func WithCtx(ctx context.Context) *slog.Logger {
	return L().With("ctx", ctx)
}

// ---- prettyHandler: 人类可读的文本 handler ----

// ANSI 颜色.
const (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorGray   = "\033[90m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// prettyHandler 自定义 slog.Handler, 输出可读彩色日志.
//
// 格式: HH:MM:SS LEVEL module message key=value key=value
type prettyHandler struct {
	w        io.Writer
	color    bool
	minLvl   slog.Level
	mu       *sync.Mutex
	preAttrs []slog.Attr
}

// newPrettyHandler 创建 handler. w 为 nil 时不会输出 (用于文件未配置场景).
func newPrettyHandler(w io.Writer, color bool, lvl slog.Level) *prettyHandler {
	return &prettyHandler{
		w:      w,
		color:  color,
		minLvl: lvl,
		mu:     &sync.Mutex{},
	}
}

// Enabled 实现 slog.Handler.
func (h *prettyHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.minLvl
}

// Handle 实现 slog.Handler.
func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	if h.w == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	// 收集所有 attrs (pre + record)
	module := ""
	var kvs []string
	collect := func(a slog.Attr) bool {
		if a.Key == "module" {
			module = a.Value.String()
			return true
		}
		if a.Key == "time" {
			return true // 用 r.Time
		}
		if a.Key == "level" {
			return true // 用 r.Level
		}
		if a.Key == "msg" {
			return true // 用 r.Message
		}
		kvs = append(kvs, formatKV(a.Key, a.Value))
		return true
	}
	for _, a := range h.preAttrs {
		collect(a)
	}
	r.Attrs(collect)

	// 时间 HH:MM:SS.mmm
	ts := r.Time.Local().Format("15:04:05.000")

	// 级别 4 字符 (DEBU/INFO/WARN/ERRO)
	lvlStr := levelLabel(r.Level)
	lvlColored := colorize(lvlStr, levelColor(r.Level), h.color)

	// 主消息
	msgColored := colorize(r.Message, colorBold, h.color)

	// 拼接
	var b strings.Builder
	b.WriteString(colorize(ts, colorGray, h.color))
	b.WriteByte(' ')
	b.WriteString(lvlColored)
	b.WriteByte(' ')
	if module != "" {
		b.WriteString(colorize(module, colorCyan, h.color))
		b.WriteByte(' ')
	}
	b.WriteString(msgColored)
	if len(kvs) > 0 {
		b.WriteString("  ")
		b.WriteString(strings.Join(kvs, " "))
	}
	b.WriteByte('\n')

	_, err := io.WriteString(h.w, b.String())
	return err
}

// WithAttrs 实现 slog.Handler (支持 With(...).With(...)).
func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := append([]slog.Attr{}, h.preAttrs...)
	merged = append(merged, attrs...)
	return &prettyHandler{
		w:        h.w,
		color:    h.color,
		minLvl:   h.minLvl,
		mu:       h.mu,
		preAttrs: merged,
	}
}

// WithGroup 实现 slog.Handler.
func (h *prettyHandler) WithGroup(name string) slog.Handler {
	// 简化: 不实现嵌套 group, 直接返回自身.
	return h
}

func levelLabel(l slog.Level) string {
	if logx.IsZH() {
		switch l {
		case slog.LevelDebug:
			return "调试"
		case slog.LevelInfo:
			return "信息"
		case slog.LevelWarn:
			return "警告"
		case slog.LevelError:
			return "错误"
		}
	}
	switch l {
	case slog.LevelDebug:
		return "DEBU"
	case slog.LevelInfo:
		return "INFO"
	case slog.LevelWarn:
		return "WARN"
	case slog.LevelError:
		return "ERRO"
	default:
		s := strings.ToUpper(l.String())
		if len(s) > 4 {
			return s[:4]
		}
		return fmt.Sprintf("%-4s", s)
	}
}

func levelColor(l slog.Level) string {
	switch l {
	case slog.LevelDebug:
		return colorGray
	case slog.LevelInfo:
		return colorGreen
	case slog.LevelWarn:
		return colorYellow
	case slog.LevelError:
		return colorRed
	default:
		return colorReset
	}
}

func colorize(s, color string, on bool) string {
	if !on || color == "" {
		return s
	}
	return color + s + colorReset
}

func formatKV(key string, v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\"") {
			return fmt.Sprintf("%s=%q", key, s)
		}
		return key + "=" + s
	case slog.KindInt64:
		return fmt.Sprintf("%s=%d", key, v.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%s=%d", key, v.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%s=%g", key, v.Float64())
	case slog.KindBool:
		return fmt.Sprintf("%s=%t", key, v.Bool())
	case slog.KindDuration:
		return fmt.Sprintf("%s=%s", key, time.Duration(v.Int64()).String())
	case slog.KindTime:
		t := v.Time().Local().Format("15:04:05")
		return fmt.Sprintf("%s=%s", key, t)
	default:
		return fmt.Sprintf("%s=%v", key, v.Any())
	}
}

// multiHandler 多 handler 组合.
type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, lvl) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(multiHandler, len(m))
	for i, h := range m {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	out := make(multiHandler, len(m))
	for i, h := range m {
		out[i] = h.WithGroup(name)
	}
	return out
}

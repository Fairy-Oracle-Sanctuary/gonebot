// Package logx 日志 i18n 支持.
//
// 自动根据系统语言选择中/英文日志消息.
// 通过 WrapHandler 包装 slog.Handler, 在输出层做消息翻译.
// 无需修改任何现有 slog 调用.
//
// 用法:
//
//	handler := logx.WrapHandler(slog.NewTextHandler(os.Stdout, nil))
//	slog.SetDefault(slog.New(handler))
package logx

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Locale 环境语言.
type Locale string

const (
	EN Locale = "en"
	ZH Locale = "zh"
)

var currentLocale = detectLocale()

func init() {
	currentLocale = detectLocale()
}

// IsZH 返回当前是否为中文环境.
func IsZH() bool { return currentLocale == ZH }

// LocaleTag 返回当前语言标签.
func LocaleTag() Locale { return currentLocale }

func detectLocale() Locale {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.ToLower(os.Getenv(key))
		if strings.HasPrefix(v, "zh") {
			return ZH
		}
	}
	return EN
}

// T 翻译消息. 未找到时返回原文.
func T(key string) string {
	if !IsZH() {
		return key
	}
	if translated, ok := zhMessages[key]; ok {
		return translated
	}
	return key
}

// Printf 带 i18n 的 stderr 输出 (用于日志初始化前).
func Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprint(os.Stderr, msg)
}

// i18nHandler 包装 slog.Handler, 在 Handle 中翻译 msg.
type i18nHandler struct {
	next slog.Handler
}

// WrapHandler 包装 handler, 使其输出时自动翻译消息.
// 返回包装后的 handler, 可链式嵌套.
func WrapHandler(next slog.Handler) slog.Handler {
	if !IsZH() {
		return next // 非中文环境, 不翻译
	}
	return &i18nHandler{next: next}
}

func (h *i18nHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *i18nHandler) Handle(ctx context.Context, r slog.Record) error {
	if translated, ok := zhMessages[r.Message]; ok {
		r.Message = translated
	}
	return h.next.Handle(ctx, r)
}

func (h *i18nHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &i18nHandler{next: h.next.WithAttrs(attrs)}
}

func (h *i18nHandler) WithGroup(name string) slog.Handler {
	return &i18nHandler{next: h.next.WithGroup(name)}
}

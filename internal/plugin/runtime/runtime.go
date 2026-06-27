// Package runtime 定义插件运行时抽象.
package runtime

import (
	"context"

	"neobot/core/internal/bot"
	"neobot/core/internal/permission"
	"neobot/core/internal/plugin/deps"
)

// RegistrySubset Runtime 操作 Registry 所需的最小接口.
type RegistrySubset interface {
	RegisterCommand(pluginName, name string, aliases []string, required permission.Level, h CommandHandler)
	RegisterMessageHook(pluginName string, h MessageHandler)
	RegisterNoticeHook(pluginName, noticeType string, h NoticeHandler)
}

// CommandHandler 命令处理函数.
type CommandHandler func(args []string) any

// MessageHandler 全局消息 hook.
type MessageHandler func(text string) any

// NoticeHandler 通知 hook.
type NoticeHandler func(noticeType string) any

// RequestHandler 请求事件处理函数签名.
type RequestHandler func(requestType string, flag string, userID int64, groupID int64, comment string) any

// Host 提供给 Runtime 的宿主能力.
type Host struct {
	Bot      *bot.Bot
	Registry RegistrySubset
	Perm     *permission.Checker
	Redis    RedisService
	MySQL    MySQLService
	Renderer RendererService
	Deps     *deps.Manager // 依赖管理器 (可选)
	EventCtx *EventCtx     // 当前事件上下文 (Router 分发前设置)
}

// EventCtx 当前处理中的事件上下文.
type EventCtx struct {
	UserID      int64
	GroupID     int64
	MessageType string
	RawMessage  string
	MessageID   int64
	SelfID      int64
	Message     []Seg // 当前消息的消息段 (图片/文本/at 等)
}

// Seg 消息段 (避免循环引用 event 包).
type Seg struct {
	Type string
	Data map[string]any
}

// RedisService Lua 可见的 Redis 接口.
type RedisService interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttlSec int) error
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	Incr(ctx context.Context, key string) (int64, error)
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key string, values map[string]any) error
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	LPush(ctx context.Context, key string, values ...any) error
	RPush(ctx context.Context, key string, values ...any) error
	LPop(ctx context.Context, key string) (string, error)
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	LLen(ctx context.Context, key string) (int64, error)
}

// MySQLService Lua 可见的 MySQL 接口.
type MySQLService interface {
	Query(ctx context.Context, sql string, args ...any) ([]map[string]any, error)
	QueryOne(ctx context.Context, sql string, args ...any) (map[string]any, error)
	Exec(ctx context.Context, sql string, args ...any) (int64, error)
}

// RendererService Lua 可见的图片渲染接口.
type RendererService interface {
	RenderHTML(ctx context.Context, html string, width, quality int) ([]byte, error)
	RenderURL(ctx context.Context, url string, width, quality int) ([]byte, error)
	RenderTemplate(ctx context.Context, tpl string, data map[string]any, width, quality int) ([]byte, error)
}

// Runtime 插件运行时抽象.
type Runtime interface {
	Name() string
	Load(ctx context.Context, host *Host, dir, entry string, meta *Metadata) error
	Unload(pluginName string) error
	Reload(ctx context.Context, host *Host, dir, entry string, meta *Metadata) error
	Close() error
}

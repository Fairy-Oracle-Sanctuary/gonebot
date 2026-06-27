package plugin

import (
	"strings"
	"sync"

	"neobot/core/internal/permission"
	"neobot/core/internal/plugin/runtime"
)

// CommandHandler 命令处理函数 (runtime.CommandHandler 的别名).
type CommandHandler = runtime.CommandHandler

// MessageHandler 全局消息 hook.
type MessageHandler = runtime.MessageHandler

// NoticeHandler 通知 hook.
type NoticeHandler = runtime.NoticeHandler

// commandEntry.
type commandEntry struct {
	Name     string
	Aliases  []string
	Plugin   string
	Required permission.Level
	Handler  CommandHandler
}

// Registry 线程安全的命令/事件注册表.
type Registry struct {
	mu sync.RWMutex

	commands    map[string]*commandEntry
	aliases     map[string]string
	msgHooks    map[string][]MessageHandler
	noticeHooks map[string]map[string][]NoticeHandler
	reqHooks    map[string]map[string][]RequestHandler
	plugins     map[string]*PluginMeta
}

// NewRegistry 创建注册表.
func NewRegistry() *Registry {
	return &Registry{
		commands:    make(map[string]*commandEntry),
		aliases:     make(map[string]string),
		msgHooks:    make(map[string][]MessageHandler),
		noticeHooks: make(map[string]map[string][]NoticeHandler),
		reqHooks:    make(map[string]map[string][]RequestHandler),
		plugins:     make(map[string]*PluginMeta),
	}
}

// RegisterCommand 注册命令.
func (r *Registry) RegisterCommand(pluginName, name string, aliases []string, required permission.Level, h CommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[name] = &commandEntry{
		Name: name, Aliases: aliases, Plugin: pluginName,
		Required: required, Handler: h,
	}
	for _, a := range aliases {
		r.aliases[a] = name
	}
	r.touchPlugin(pluginName)
}

// RegisterMessageHook 注册全局消息 hook.
func (r *Registry) RegisterMessageHook(pluginName string, h MessageHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgHooks[pluginName] = append(r.msgHooks[pluginName], h)
	r.touchPlugin(pluginName)
}

// RegisterNoticeHook 注册通知 hook.
func (r *Registry) RegisterNoticeHook(pluginName, noticeType string, h NoticeHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.noticeHooks[pluginName] == nil {
		r.noticeHooks[pluginName] = make(map[string][]NoticeHandler)
	}
	r.noticeHooks[pluginName][noticeType] = append(r.noticeHooks[pluginName][noticeType], h)
	r.touchPlugin(pluginName)
}

// SetPluginMeta 写入/更新插件展示元信息.
func (r *Registry) SetPluginMeta(meta *PluginMeta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[meta.Name] = meta
}

// LookupCommand 按命令名 (含 alias) 查找.
func (r *Registry) LookupCommand(name string) (*commandEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.commands[name]; ok {
		return e, false
	}
	if canonical, ok := r.aliases[name]; ok {
		if e, ok := r.commands[canonical]; ok {
			return e, true
		}
	}
	return nil, false
}

// AllMessageHooks 返回所有插件的消息 hook.
func (r *Registry) AllMessageHooks() []MessageHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []MessageHandler
	for _, list := range r.msgHooks {
		out = append(out, list...)
	}
	return out
}

// AllNoticeHooks 返回指定 notice 类型的全部 hook.
func (r *Registry) AllNoticeHooks(noticeType string) []NoticeHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []NoticeHandler
	for _, typeMap := range r.noticeHooks {
		out = append(out, typeMap[noticeType]...)
	}
	return out
}

// Plugins 返回所有插件元信息.
func (r *Registry) Plugins() map[string]*PluginMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]*PluginMeta, len(r.plugins))
	for k, v := range r.plugins {
		out[k] = v
	}
	return out
}

// UnregisterByPlugin 移除指定插件的所有注册.
func (r *Registry) UnregisterByPlugin(pluginName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for n, e := range r.commands {
		if e.Plugin == pluginName {
			delete(r.commands, n)
		}
	}
	for a, canonical := range r.aliases {
		if e, ok := r.commands[canonical]; ok && e.Plugin == pluginName {
			delete(r.aliases, a)
		}
	}
	delete(r.msgHooks, pluginName)
	delete(r.noticeHooks, pluginName)
	delete(r.plugins, pluginName)
}

// Clear 清空所有 (保留内部 map).
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = make(map[string]*commandEntry)
	r.aliases = make(map[string]string)
	r.msgHooks = make(map[string][]MessageHandler)
	r.noticeHooks = make(map[string]map[string][]NoticeHandler)
	r.plugins = make(map[string]*PluginMeta)
}

func (r *Registry) touchPlugin(name string) {
	if _, ok := r.plugins[name]; !ok {
		r.plugins[name] = &PluginMeta{Name: name}
	}
}

// 抑制未使用告警.
var _ = strings.TrimSpace

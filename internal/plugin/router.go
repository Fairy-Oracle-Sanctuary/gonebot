package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"neobot/core/internal/bot"
	"neobot/core/internal/event"
	"neobot/core/internal/permission"
	"neobot/core/internal/plugin/runtime"
)

// Router 事件分发.
type Router struct {
	registry *Registry
	bot      *bot.Bot
	perm     *permission.Checker
	host     *runtime.Host
	logger   *slog.Logger
}

// NewRouter 创建路由.
func NewRouter(reg *Registry, b *bot.Bot, perm *permission.Checker, host *runtime.Host) *Router {
	return &Router{
		registry: reg,
		bot:      b,
		perm:     perm,
		host:     host,
		logger:   slog.Default().With("module", "plugin.router"),
	}
}

// Dispatch 事件分发入口.
func (r *Router) Dispatch(ctx context.Context, ev *event.Any) {
	if ev == nil {
		return
	}
	switch ev.Type {
	case event.PostMessage:
		m := ev.Message
		if m == nil {
			return
		}
		if m.UserID != 0 && m.SelfID != 0 && m.UserID == m.SelfID {
			return
		}
		r.logger.Debug("dispatch message",
			"message_type", string(m.MessageType),
			"user_id", m.UserID,
			"group_id", m.GroupID,
			"raw", event.PlainText(m.Message),
		)

		// 设置事件上下文, 供 Lua SDK 的 neobot.event 访问
		if r.host != nil {
			segs := make([]runtime.Seg, len(m.Message))
			for i, s := range m.Message {
				segs[i] = runtime.Seg{Type: string(s.Type), Data: s.Data}
			}
			r.host.EventCtx = &runtime.EventCtx{
				UserID:      m.UserID,
				GroupID:     m.GroupID,
				MessageType: string(m.MessageType),
				RawMessage:  m.RawMessage,
				MessageID:   m.MessageID,
				SelfID:      m.SelfID,
				Message:     segs,
			}
		}

		text := event.PlainText(m.Message)

		// 全局 hook
		for _, h := range r.registry.AllMessageHooks() {
			if reply := h(text); reply != nil {
				r.sendReply(ctx, m, reply)
			}
		}

		// 命令
		cmd := firstWord(text)
		if cmd == "" {
			return
		}
		if entry, _ := r.registry.LookupCommand(cmd); entry != nil {
			r.logger.Info("command matched", "cmd", cmd, "plugin", entry.Plugin, "args", parseArgs(text))
			// 权限检查
			if !r.checkPerm(m, entry) {
				r.logger.Warn("permission denied",
					"plugin", entry.Plugin, "command", entry.Name,
					"user_id", m.UserID, "group_id", m.GroupID)
				r.sendReply(ctx, m, "权限不足")
				return
			}
			if reply := entry.Handler(parseArgs(text)); reply != nil {
				r.sendReply(ctx, m, reply)
			} else {
				r.logger.Debug("command handler returned nil", "cmd", cmd)
			}
		} else {
			r.logger.Debug("command not found in registry", "cmd", cmd)
		}

	case event.PostNotice:
		n := ev.Notice
		if n == nil {
			return
		}
		r.logger.Debug("dispatch notice",
			"notice_type", string(n.NoticeType),
			"user_id", n.UserID,
			"group_id", n.GroupID,
		)
		for _, h := range r.registry.AllNoticeHooks(string(n.NoticeType)) {
			_ = h(string(n.NoticeType))
		}
	}
}

// checkPerm 检查用户权限. 无 checker 或 required=User 直接通过.
func (r *Router) checkPerm(m *event.MessageEvent, entry *commandEntry) bool {
	if r.perm == nil || entry.Required == permission.User {
		return true
	}
	role := ""
	if m.IsGroup() {
		role = m.Sender.Role
	}
	return r.perm.Check(m.UserID, m.GroupID, role, entry.Required)
}

func (r *Router) sendReply(ctx context.Context, m *event.MessageEvent, reply any) {
	switch v := reply.(type) {
	case string:
		if v == "" {
			return
		}
		r.logger.Info("replying", "to_user", m.UserID, "to_group", m.GroupID, "text", v)
		_, _ = r.bot.Reply(ctx, m, v)
	case event.MessageSegment:
		r.logger.Info("replying", "to_user", m.UserID, "to_group", m.GroupID, "type", v.Type)
		_, _ = r.bot.Reply(ctx, m, v)
	case []event.MessageSegment:
		r.logger.Info("replying", "to_user", m.UserID, "to_group", m.GroupID, "segs", len(v))
		_, _ = r.bot.Reply(ctx, m, v)
	default:
		r.logger.Info("replying", "to_user", m.UserID, "to_group", m.GroupID)
		_, _ = r.bot.Reply(ctx, m, fmt.Sprintf("%v", v))
	}
}

func firstWord(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	word := fields[0]
	// 去掉常见命令前缀 / #
	for len(word) > 0 && (word[0] == '/' || word[0] == '#') {
		word = word[1:]
	}
	return word
}

func parseArgs(text string) []string {
	idx := strings.IndexAny(text, " \t")
	if idx < 0 {
		return nil
	}
	rest := strings.TrimSpace(text[idx+1:])
	if rest == "" {
		return nil
	}
	return strings.Fields(rest)
}

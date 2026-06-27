// Lua SDK 注入.
package runtime

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"

	"neobot/core/internal/permission"
)

// injectSDK 在 L 中创建全局 neobot 表.
func injectSDK(L *lua.LState, host *Host, state *luaState, meta *Metadata) {
	neobot := L.NewTable()

	// ---------- neobot.log ----------
	logT := L.NewTable()
	logT.RawSetString("debug", L.NewFunction(func(L *lua.LState) int {
		logOf(state).Debug(luaStr(L.CheckAny(1)))
		return 0
	}))
	logT.RawSetString("info", L.NewFunction(func(L *lua.LState) int {
		logOf(state).Info(luaStr(L.CheckAny(1)))
		return 0
	}))
	logT.RawSetString("warn", L.NewFunction(func(L *lua.LState) int {
		logOf(state).Warn(luaStr(L.CheckAny(1)))
		return 0
	}))
	logT.RawSetString("error", L.NewFunction(func(L *lua.LState) int {
		logOf(state).Error(luaStr(L.CheckAny(1)))
		return 0
	}))
	neobot.RawSetString("log", logT)

	// ---------- neobot.config ----------
	configT := L.NewTable()
	configT.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		key := luaStr(L.CheckAny(1))
		def := L.Get(2)
		if v, ok := meta.Config[key]; ok {
			L.Push(goToLua(L, v))
			return 1
		}
		if def.Type() == lua.LTNil {
			L.Push(lua.LNil)
		} else {
			L.Push(def)
		}
		return 1
	}))
	neobot.RawSetString("config", configT)

	// ---------- neobot.bot ----------
	botT := L.NewTable()
	botT.RawSetString("call_api", L.NewFunction(func(L *lua.LState) int {
		action := luaStr(L.CheckAny(1))
		var params map[string]any
		if p := L.Get(2); p.Type() == lua.LTTable {
			params = luaTableToMap(p.(*lua.LTable))
		}
		slog.Debug("lua call_api", "action", action, "params", params)
		resp, err := host.Bot.CallAPI(context.Background(), action, params)
		if err != nil {
			slog.Warn("lua call_api failed", "action", action, "err", err)
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		slog.Debug("lua call_api done", "action", action, "status", resp.Status, "retcode", resp.RetCode)
		out := L.NewTable()
		out.RawSetString("status", lua.LString(resp.Status))
		out.RawSetString("retcode", lua.LNumber(resp.RetCode))
		out.RawSetString("msg", lua.LString(resp.Msg))
		out.RawSetString("data", lua.LString(string(resp.Data)))
		L.Push(out)
		return 1
	}))
	botT.RawSetString("self_id", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(host.Bot.SelfID()))
		return 1
	}))
	// Bot 快捷方法
	botT.RawSetString("send_private_msg", L.NewFunction(func(L *lua.LState) int {
		userID := int64(lua.LVAsNumber(L.CheckAny(1)))
		msg := luaToGo(L.Get(2))
		msgID, err := host.Bot.SendPrivateMsg(context.Background(), userID, msg)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(msgID))
		return 1
	}))
	botT.RawSetString("send_group_msg", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		msg := luaToGo(L.Get(2))
		msgID, err := host.Bot.SendGroupMsg(context.Background(), groupID, msg)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(msgID))
		return 1
	}))
	botT.RawSetString("send_like", L.NewFunction(func(L *lua.LState) int {
		userID := int64(lua.LVAsNumber(L.CheckAny(1)))
		times := int(lua.LVAsNumber(L.Get(2)))
		if times == 0 {
			times = 1
		}
		err := host.Bot.SendLike(context.Background(), userID, times)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))
	botT.RawSetString("delete_msg", L.NewFunction(func(L *lua.LState) int {
		msgID := int64(lua.LVAsNumber(L.CheckAny(1)))
		err := host.Bot.DeleteMsg(context.Background(), msgID)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))
	botT.RawSetString("get_group_list", L.NewFunction(func(L *lua.LState) int {
		list, err := host.Bot.GetGroupList(context.Background())
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		out := L.NewTable()
		for i, g := range list {
			gt := L.NewTable()
			for k, v := range g {
				gt.RawSetString(k, goToLua(L, v))
			}
			out.RawSetInt(i+1, gt)
		}
		L.Push(out)
		return 1
	}))
	botT.RawSetString("get_group_info", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		resp, err := host.Bot.GetGroupInfo(context.Background(), groupID)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("get_group_member_list", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		resp, err := host.Bot.GetGroupMemberList(context.Background(), groupID)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("get_group_member_info", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		userID := int64(lua.LVAsNumber(L.CheckAny(2)))
		resp, err := host.Bot.GetGroupMemberInfo(context.Background(), groupID, userID)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("group_kick", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		userID := int64(lua.LVAsNumber(L.CheckAny(2)))
		reject := lua.LVAsBool(L.Get(3))
		err := host.Bot.GroupKick(context.Background(), groupID, userID, reject)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))
	botT.RawSetString("group_ban", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		userID := int64(lua.LVAsNumber(L.CheckAny(2)))
		duration := int(lua.LVAsNumber(L.Get(3)))
		if duration == 0 {
			duration = 1800
		}
		err := host.Bot.GroupBan(context.Background(), groupID, userID, duration)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))
	botT.RawSetString("set_group_card", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		userID := int64(lua.LVAsNumber(L.CheckAny(2)))
		card := luaStr(L.Get(3))
		err := host.Bot.SetGroupCard(context.Background(), groupID, userID, card)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))
	botT.RawSetString("set_group_whole_ban", L.NewFunction(func(L *lua.LState) int {
		groupID := int64(lua.LVAsNumber(L.CheckAny(1)))
		enable := lua.LVAsBool(L.Get(2))
		err := host.Bot.SetGroupWholeBan(context.Background(), groupID, enable)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(true))
		return 1
	}))
	botT.RawSetString("get_login_info", L.NewFunction(func(L *lua.LState) int {
		resp, err := host.Bot.GetLoginInfo(context.Background())
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("get_stranger_info", L.NewFunction(func(L *lua.LState) int {
		userID := int64(lua.LVAsNumber(L.CheckAny(1)))
		resp, err := host.Bot.GetStrangerInfo(context.Background(), userID)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("get_friend_list", L.NewFunction(func(L *lua.LState) int {
		resp, err := host.Bot.GetFriendList(context.Background())
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("can_send_image", L.NewFunction(func(L *lua.LState) int {
		resp, err := host.Bot.CanSendImage(context.Background())
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("can_send_record", L.NewFunction(func(L *lua.LState) int {
		resp, err := host.Bot.CanSendRecord(context.Background())
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	botT.RawSetString("get_image", L.NewFunction(func(L *lua.LState) int {
		file := luaStr(L.CheckAny(1))
		resp, err := host.Bot.GetImage(context.Background(), file)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(resp.Data)))
		return 1
	}))
	neobot.RawSetString("bot", botT)

	// ---------- neobot.perm (权限检查) ----------
	permT := L.NewTable()
	permT.RawSetString("check", L.NewFunction(func(L *lua.LState) int {
		userID := int64(lua.LVAsNumber(L.CheckAny(1)))
		groupID := int64(lua.LVAsNumber(L.Get(2)))
		role := luaStr(L.Get(3))
		required := permission.ParseLevelSimple(luaStr(L.Get(4)))
		if host.Perm == nil {
			L.Push(lua.LBool(false))
			return 1
		}
		L.Push(lua.LBool(host.Perm.Check(userID, groupID, role, required)))
		return 1
	}))
	permT.RawSetString("is_super", L.NewFunction(func(L *lua.LState) int {
		userID := int64(lua.LVAsNumber(L.CheckAny(1)))
		if host.Perm == nil {
			L.Push(lua.LBool(false))
			return 1
		}
		L.Push(lua.LBool(host.Perm.IsSuperUser(userID)))
		return 1
	}))
	neobot.RawSetString("perm", permT)

	// ---------- neobot.seg (消息段构造) ----------
	neobot.RawSetString("seg", buildSegT(L))

	// ---------- neobot.register ----------
	regT := L.NewTable()
	regT.RawSetString("command", L.NewFunction(func(L *lua.LState) int {
		name := luaStr(L.CheckAny(1))
		permStr := luaStr(L.CheckAny(2))
		fn := L.CheckFunction(3)

		required := permission.ParseLevelSimple(permStr)

		var aliases []string
		if opt := L.Get(4); opt.Type() == lua.LTTable {
			if t := opt.(*lua.LTable); t != nil {
				if a := t.RawGetString("aliases"); a.Type() == lua.LTTable {
					a.(*lua.LTable).ForEach(func(_, v lua.LValue) {
						s := luaStr(v)
						if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
							s = s[1 : len(s)-1]
						}
						aliases = append(aliases, s)
					})
				}
			}
		}

		host.Registry.RegisterCommand(meta.Name, name, aliases, required, func(args []string) any {
			return callLua(L, fn, args)
		})
		return 0
	}))

	regT.RawSetString("on_message", L.NewFunction(func(L *lua.LState) int {
		fn := L.CheckFunction(1)
		host.Registry.RegisterMessageHook(meta.Name, func(text string) any {
			// message hook 收到纯文本, 直接传字符串不包 table
			return callLuaSingle(L, fn, text)
		})
		return 0
	}))

	regT.RawSetString("on_notice", L.NewFunction(func(L *lua.LState) int {
		noticeType := luaStr(L.CheckAny(1))
		fn := L.CheckFunction(2)
		host.Registry.RegisterNoticeHook(meta.Name, noticeType, func(t string) any {
			return callLua(L, fn, []string{t})
		})
		return 0
	}))
	neobot.RawSetString("register", regT)

	// ---------- neobot.event (当前事件上下文, 只读) ----------
	neobot.RawSetString("event", buildEventT(L, host))

	// ---------- neobot.redis ----------
	neobot.RawSetString("redis", buildRedisT(L, host))

	// ---------- neobot.mysql ----------
	neobot.RawSetString("mysql", buildMySQLT(L, host))

	// ---------- neobot.render ----------
	neobot.RawSetString("render", buildRenderT(L, host))

	// ---------- neobot.util ----------
	utilT := L.NewTable()
	utilT.RawSetString("now", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LNumber(time.Now().Unix()))
		return 1
	}))
	utilT.RawSetString("date", L.NewFunction(func(L *lua.LState) int {
		fmtStr := luaStr(L.CheckAny(1))
		if fmtStr == "" {
			fmtStr = "%Y-%m-%d %H:%M:%S"
		}
		L.Push(lua.LString(time.Now().Format(convertDateFmt(fmtStr))))
		return 1
	}))
	utilT.RawSetString("sleep", L.NewFunction(func(L *lua.LState) int {
		ms := int(lua.LVAsNumber(L.CheckAny(1)))
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return 0
	}))
	utilT.RawSetString("http_get", L.NewFunction(func(L *lua.LState) int {
		url := luaStr(L.CheckAny(1))
		resp, err := httpGet(url)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(resp))
		return 1
	}))
	utilT.RawSetString("http_post", L.NewFunction(func(L *lua.LState) int {
		url := luaStr(L.CheckAny(1))
		body := luaStr(L.Get(2))
		contentType := luaStr(L.Get(3))
		if contentType == "" {
			contentType = "application/json"
		}
		resp, err := httpPost(url, contentType, body)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(resp))
		return 1
	}))
	utilT.RawSetString("base64_encode", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(base64.StdEncoding.EncodeToString([]byte(luaStr(L.CheckAny(1))))))
		return 1
	}))
	utilT.RawSetString("base64_decode", L.NewFunction(func(L *lua.LState) int {
		data, err := base64.StdEncoding.DecodeString(luaStr(L.CheckAny(1)))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(data)))
		return 1
	}))
	utilT.RawSetString("md5", L.NewFunction(func(L *lua.LState) int {
		h := md5.Sum([]byte(luaStr(L.CheckAny(1))))
		L.Push(lua.LString(hex.EncodeToString(h[:])))
		return 1
	}))
	utilT.RawSetString("sha256", L.NewFunction(func(L *lua.LState) int {
		h := sha256.Sum256([]byte(luaStr(L.CheckAny(1))))
		L.Push(lua.LString(hex.EncodeToString(h[:])))
		return 1
	}))
	utilT.RawSetString("json_encode", L.NewFunction(func(L *lua.LState) int {
		v := L.Get(1)
		data, err := json.Marshal(luaToGo(v))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(data)))
		return 1
	}))
	utilT.RawSetString("json_decode", L.NewFunction(func(L *lua.LState) int {
		s := luaStr(L.CheckAny(1))
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLua(L, v))
		return 1
	}))
	neobot.RawSetString("util", utilT)

	L.SetGlobal("neobot", neobot)
}

// ---- Redis SDK ----

func buildRedisT(L *lua.LState, host *Host) *lua.LTable {
	t := L.NewTable()
	if host.Redis == nil {
		t.RawSetString("available", lua.LBool(false))
		return t
	}
	t.RawSetString("available", lua.LBool(true))

	ctx := context.Background()

	t.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		v, err := host.Redis.Get(ctx, luaStr(L.CheckAny(1)))
		pushKV(L, v, err)
		return 2
	}))
	t.RawSetString("set", L.NewFunction(func(L *lua.LState) int {
		key := luaStr(L.CheckAny(1))
		val := luaStr(L.CheckAny(2))
		ttl := int(lua.LVAsNumber(L.Get(3)))
		err := host.Redis.Set(ctx, key, val, ttl)
		pushErr(L, err)
		return 1
	}))
	t.RawSetString("del", L.NewFunction(func(L *lua.LState) int {
		n := L.GetTop()
		keys := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			keys = append(keys, luaStr(L.Get(i)))
		}
		pushErr(L, host.Redis.Del(ctx, keys...))
		return 1
	}))
	t.RawSetString("exists", L.NewFunction(func(L *lua.LState) int {
		v, err := host.Redis.Exists(ctx, luaStr(L.CheckAny(1)))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LBool(v))
		return 1
	}))
	t.RawSetString("incr", L.NewFunction(func(L *lua.LState) int {
		v, err := host.Redis.Incr(ctx, luaStr(L.CheckAny(1)))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(v))
		return 1
	}))
	t.RawSetString("hget", L.NewFunction(func(L *lua.LState) int {
		v, err := host.Redis.HGet(ctx, luaStr(L.CheckAny(1)), luaStr(L.CheckAny(2)))
		pushKV(L, v, err)
		return 2
	}))
	t.RawSetString("hset", L.NewFunction(func(L *lua.LState) int {
		key := luaStr(L.CheckAny(1))
		tbl := L.CheckTable(2)
		m := luaTableToMap(tbl)
		pushErr(L, host.Redis.HSet(ctx, key, m))
		return 1
	}))
	t.RawSetString("hgetall", L.NewFunction(func(L *lua.LState) int {
		v, err := host.Redis.HGetAll(ctx, luaStr(L.CheckAny(1)))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		out := L.NewTable()
		for k, val := range v {
			out.RawSetString(k, lua.LString(val))
		}
		L.Push(out)
		return 1
	}))
	t.RawSetString("lpush", L.NewFunction(func(L *lua.LState) int {
		key := luaStr(L.CheckAny(1))
		n := L.GetTop()
		vals := make([]any, 0, n-1)
		for i := 2; i <= n; i++ {
			vals = append(vals, luaStr(L.Get(i)))
		}
		pushErr(L, host.Redis.LPush(ctx, key, vals...))
		return 1
	}))
	t.RawSetString("rpush", L.NewFunction(func(L *lua.LState) int {
		key := luaStr(L.CheckAny(1))
		n := L.GetTop()
		vals := make([]any, 0, n-1)
		for i := 2; i <= n; i++ {
			vals = append(vals, luaStr(L.Get(i)))
		}
		pushErr(L, host.Redis.RPush(ctx, key, vals...))
		return 1
	}))
	t.RawSetString("lpop", L.NewFunction(func(L *lua.LState) int {
		v, err := host.Redis.LPop(ctx, luaStr(L.CheckAny(1)))
		pushKV(L, v, err)
		return 2
	}))
	t.RawSetString("lrange", L.NewFunction(func(L *lua.LState) int {
		key := luaStr(L.CheckAny(1))
		start := int64(lua.LVAsNumber(L.CheckAny(2)))
		stop := int64(lua.LVAsNumber(L.CheckAny(3)))
		v, err := host.Redis.LRange(ctx, key, start, stop)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		out := L.NewTable()
		for i, s := range v {
			out.RawSetInt(i+1, lua.LString(s))
		}
		L.Push(out)
		return 1
	}))
	t.RawSetString("llen", L.NewFunction(func(L *lua.LState) int {
		v, err := host.Redis.LLen(ctx, luaStr(L.CheckAny(1)))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(v))
		return 1
	}))
	return t
}

// ---- MySQL SDK ----

func buildMySQLT(L *lua.LState, host *Host) *lua.LTable {
	t := L.NewTable()
	if host.MySQL == nil {
		t.RawSetString("available", lua.LBool(false))
		return t
	}
	t.RawSetString("available", lua.LBool(true))

	ctx := context.Background()

	t.RawSetString("query", L.NewFunction(func(L *lua.LState) int {
		sqlStr := luaStr(L.CheckAny(1))
		n := L.GetTop()
		args := make([]any, 0, n-1)
		for i := 2; i <= n; i++ {
			args = append(args, luaStr(L.Get(i)))
		}
		rows, err := host.MySQL.Query(ctx, sqlStr, args...)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		out := L.NewTable()
		for i, r := range rows {
			rowT := L.NewTable()
			for k, v := range r {
				rowT.RawSetString(k, goToLua(L, v))
			}
			out.RawSetInt(i+1, rowT)
		}
		L.Push(out)
		return 1
	}))
	t.RawSetString("query_one", L.NewFunction(func(L *lua.LState) int {
		sqlStr := luaStr(L.CheckAny(1))
		n := L.GetTop()
		args := make([]any, 0, n-1)
		for i := 2; i <= n; i++ {
			args = append(args, luaStr(L.Get(i)))
		}
		row, err := host.MySQL.QueryOne(ctx, sqlStr, args...)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		out := L.NewTable()
		for k, v := range row {
			out.RawSetString(k, goToLua(L, v))
		}
		L.Push(out)
		return 1
	}))
	t.RawSetString("exec", L.NewFunction(func(L *lua.LState) int {
		sqlStr := luaStr(L.CheckAny(1))
		n := L.GetTop()
		args := make([]any, 0, n-1)
		for i := 2; i <= n; i++ {
			args = append(args, luaStr(L.Get(i)))
		}
		n64, err := host.MySQL.Exec(ctx, sqlStr, args...)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LNumber(n64))
		return 1
	}))
	return t
}

// ---- Render SDK ----

func buildRenderT(L *lua.LState, host *Host) *lua.LTable {
	t := L.NewTable()
	if host.Renderer == nil {
		t.RawSetString("available", lua.LBool(false))
		return t
	}
	t.RawSetString("available", lua.LBool(true))

	ctx := context.Background()

	t.RawSetString("html", L.NewFunction(func(L *lua.LState) int {
		html := luaStr(L.CheckAny(1))
		width := int(lua.LVAsNumber(L.Get(2)))
		if width == 0 {
			width = 800
		}
		img, err := host.Renderer.RenderHTML(ctx, html, width, 90)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString("base64://" + base64.StdEncoding.EncodeToString(img)))
		return 1
	}))
	t.RawSetString("url", L.NewFunction(func(L *lua.LState) int {
		url := luaStr(L.CheckAny(1))
		width := int(lua.LVAsNumber(L.Get(2)))
		if width == 0 {
			width = 1280
		}
		img, err := host.Renderer.RenderURL(ctx, url, width, 90)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString("base64://" + base64.StdEncoding.EncodeToString(img)))
		return 1
	}))
	t.RawSetString("template", L.NewFunction(func(L *lua.LState) int {
		tpl := luaStr(L.CheckAny(1))
		dataT := L.CheckTable(2)
		data := luaTableToMap(dataT)
		width := int(lua.LVAsNumber(L.Get(3)))
		if width == 0 {
			width = 800
		}
		img, err := host.Renderer.RenderTemplate(ctx, tpl, data, width, 90)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString("base64://" + base64.StdEncoding.EncodeToString(img)))
		return 1
	}))
	return t
}

// ---- helpers ----

// buildSegT 构建 neobot.seg 消息段构造器.
func buildSegT(L *lua.LState) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("text", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("text"))
		data := L.NewTable()
		data.RawSetString("text", lua.LString(luaStr(L.CheckAny(1))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("image", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("image"))
		data := L.NewTable()
		data.RawSetString("file", lua.LString(luaStr(L.CheckAny(1))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("at", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("at"))
		data := L.NewTable()
		data.RawSetString("qq", lua.LString(fmt.Sprintf("%d", int64(lua.LVAsNumber(L.CheckAny(1))))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("face", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("face"))
		data := L.NewTable()
		data.RawSetString("id", lua.LString(fmt.Sprintf("%d", int(lua.LVAsNumber(L.CheckAny(1))))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("reply", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("reply"))
		data := L.NewTable()
		data.RawSetString("id", lua.LString(fmt.Sprintf("%d", int64(lua.LVAsNumber(L.CheckAny(1))))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("record", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("record"))
		data := L.NewTable()
		data.RawSetString("file", lua.LString(luaStr(L.CheckAny(1))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("video", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("video"))
		data := L.NewTable()
		data.RawSetString("file", lua.LString(luaStr(L.CheckAny(1))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("json", L.NewFunction(func(L *lua.LState) int {
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("json"))
		data := L.NewTable()
		data.RawSetString("data", lua.LString(luaStr(L.CheckAny(1))))
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	t.RawSetString("node", L.NewFunction(func(L *lua.LState) int {
		userID := int64(lua.LVAsNumber(L.CheckAny(1)))
		nickname := luaStr(L.Get(2))
		msg := L.Get(3)
		seg := L.NewTable()
		seg.RawSetString("type", lua.LString("node"))
		data := L.NewTable()
		data.RawSetString("user_id", lua.LString(fmt.Sprintf("%d", userID)))
		data.RawSetString("nickname", lua.LString(nickname))
		data.RawSetString("content", msg)
		seg.RawSetString("data", data)
		L.Push(seg)
		return 1
	}))
	return t
}

// buildEventT 构建 neobot.event 只读事件上下文.
func buildEventT(L *lua.LState, host *Host) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("user_id", L.NewFunction(func(L *lua.LState) int {
		if host.EventCtx != nil {
			L.Push(lua.LNumber(host.EventCtx.UserID))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))
	t.RawSetString("group_id", L.NewFunction(func(L *lua.LState) int {
		if host.EventCtx != nil {
			L.Push(lua.LNumber(host.EventCtx.GroupID))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))
	t.RawSetString("message_type", L.NewFunction(func(L *lua.LState) int {
		if host.EventCtx != nil {
			L.Push(lua.LString(host.EventCtx.MessageType))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	}))
	t.RawSetString("raw_message", L.NewFunction(func(L *lua.LState) int {
		if host.EventCtx != nil {
			L.Push(lua.LString(host.EventCtx.RawMessage))
		} else {
			L.Push(lua.LString(""))
		}
		return 1
	}))
	t.RawSetString("message_id", L.NewFunction(func(L *lua.LState) int {
		if host.EventCtx != nil {
			L.Push(lua.LNumber(host.EventCtx.MessageID))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))
	t.RawSetString("segments", L.NewFunction(func(L *lua.LState) int {
		// 返回当前消息的完整消息段数组, 可传给 send_group_msg 等
		if host.EventCtx == nil || len(host.EventCtx.Message) == 0 {
			L.Push(L.NewTable())
			return 1
		}
		out := L.NewTable()
		for i, s := range host.EventCtx.Message {
			seg := L.NewTable()
			seg.RawSetString("type", lua.LString(s.Type))
			data := L.NewTable()
			for k, v := range s.Data {
				data.RawSetString(k, goToLua(L, v))
			}
			seg.RawSetString("data", data)
			out.RawSetInt(i+1, seg)
		}
		L.Push(out)
		return 1
	}))
	t.RawSetString("self_id", L.NewFunction(func(L *lua.LState) int {
		if host.EventCtx != nil {
			L.Push(lua.LNumber(host.EventCtx.SelfID))
		} else {
			L.Push(lua.LNumber(0))
		}
		return 1
	}))
	return t
}

func httpGet(url string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func httpPost(url, contentType, body string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, contentType, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func pushKV(L *lua.LState, v string, err error) {
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return
	}
	L.Push(lua.LString(v))
}

func pushErr(L *lua.LState, err error) {
	if err == nil {
		L.Push(lua.LNil)
		return
	}
	L.Push(lua.LString(err.Error()))
}

// callLua 调用 Lua 函数, 返回首个返回值.
// args 被包装为单个 table 传入, Lua 侧通过 args[1], args[2] 访问.
func callLua(L *lua.LState, fn *lua.LFunction, args []string) any {
	top := L.GetTop()
	defer L.SetTop(top)

	// 将所有参数打包为一个 table, 作为唯一参数传入
	argTable := L.NewTable()
	for i, a := range args {
		argTable.RawSetInt(i+1, lua.LString(a))
	}

	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, argTable); err != nil {
		logOf(nil).Error("lua call failed", "err", err.Error())
		return nil
	}
	ret := L.Get(-1)
	L.Pop(1)

	return luaToGo(ret)
}

// callLuaSingle 调用 Lua 函数, 传入单个字符串参数.
func callLuaSingle(L *lua.LState, fn *lua.LFunction, arg string) any {
	top := L.GetTop()
	defer L.SetTop(top)

	if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, lua.LString(arg)); err != nil {
		logOf(nil).Error("lua call failed", "err", err.Error())
		return nil
	}
	ret := L.Get(-1)
	L.Pop(1)

	return luaToGo(ret)
}

func luaToGo(ret lua.LValue) any {
	switch v := ret.(type) {
	case lua.LString:
		return string(v)
	case lua.LNumber:
		return float64(v)
	case lua.LBool:
		return bool(v)
	case *lua.LTable:
		// 检测是否为数组: {1:"a", 2:"b"} → []any
		if slice := luaTableToSlice(v); slice != nil {
			return slice
		}
		return luaTableToMap(v)
	default:
		return nil
	}
}

// luaTableToSlice 将 Lua 连续整数键的表转为 Go slice. 不连续或为空则返回 nil.
func luaTableToSlice(t *lua.LTable) []any {
	maxN := 0
	hasNonNum := false
	t.ForEach(func(k, _ lua.LValue) {
		n := float64(lua.LVAsNumber(k))
		if n == 0 || n != float64(int(n)) {
			hasNonNum = true
			return
		}
		if int(n) > maxN {
			maxN = int(n)
		}
	})
	if hasNonNum || maxN == 0 {
		return nil
	}
	out := make([]any, maxN)
	for i := 1; i <= maxN; i++ {
		out[i-1] = luaToGo(t.RawGetInt(i))
	}
	return out
}

func logOf(state *luaState) *slog.Logger {
	if state == nil {
		return slog.Default().With("module", "lua.sdk")
	}
	return slog.Default().With("plugin", state.pluginName, "runtime", "lua")
}

func convertDateFmt(s string) string {
	mapping := map[string]string{
		"%Y": "2006", "%y": "06", "%m": "01", "%d": "02",
		"%H": "15", "%M": "04", "%S": "05", "%I": "03",
		"%p": "PM", "%B": "January", "%b": "Jan",
		"%A": "Monday", "%a": "Mon",
	}
	out := s
	for k, v := range mapping {
		out = replaceAll(out, k, v)
	}
	return out
}

func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

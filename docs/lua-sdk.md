# Lua SDK 参考

Lua SDK 通过全局变量 `neobot`（别名 `nb`）注入到每个插件的 VM 中。

## 模块列表

| 模块 | 说明 |
|---|---|
| `neobot.log` | 日志输出 |
| `neobot.config` | 读取插件私有配置 |
| `neobot.register` | 注册命令/事件 Hook |
| `neobot.bot` | Bot API 调用 |
| `neobot.seg` | 消息段构造器 |
| `neobot.event` | 当前事件上下文 |
| `neobot.util` | 工具函数 |
| `neobot.perm` | 权限检查 |
| `neobot.redis` | Redis 操作（可选） |
| `neobot.mysql` | MySQL 操作（可选） |
| `neobot.render` | 图片渲染（可选） |

---

## neobot.log

```lua
neobot.log.debug(msg)
neobot.log.info(msg)
neobot.log.warn(msg)
neobot.log.error(msg)
```

- `msg`: 字符串，日志内容
- 输出带有 `[plugin=<name>]` 和 `[runtime=lua]` 标记

## neobot.config

```lua
local value = neobot.config.get(key, default_value)
```

读取 `plugin.toml` 中 `[config]` 段的值。键不存在时返回 `default_value`。支持 string、number、boolean 类型。

## neobot.register

### register.command

```lua
neobot.register.command(name, permission, handler, options?)
```

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 命令名（不含前缀 `/`） |
| `permission` | string | 是 | `"user"` / `"admin"` / `"superuser"` |
| `handler` | function | 是 | `function(args) end`，`args` 是字符串数组 |
| `options` | table | 否 | `{ aliases = {"别名1", "别名2"} }` |

返回值：返回字符串则自动回复，返回 nil 则不回复。

```lua
neobot.register.command("echo", "user", function(args)
    if #args == 0 then
        return "用法: /echo <内容>"
    end
    return table.concat(args, " ")
end, { aliases = { "e", "复读" } })
```

### register.on_message

```lua
neobot.register.on_message(handler)
```

每条消息都会触发所有插件的消息 Hook。第一个返回非空字符串的 Hook 会触发自动回复并停止后续 Hook。

### register.on_notice

```lua
neobot.register.on_notice(notice_type, handler)
```

支持的通知类型：

| 常量 | 说明 |
|---|---|
| `group_upload` | 群文件上传 |
| `group_admin` | 群管理员变更 |
| `group_decrease` | 群成员减少 |
| `group_increase` | 群成员增加 |
| `group_ban` | 群禁言 |
| `friend_add` | 好友添加 |
| `group_recall` | 群消息撤回 |
| `friend_recall` | 私聊消息撤回 |
| `notify` | 其他通知（戳一戳等） |

## neobot.bot

### 通用 API 调用

```lua
local result, err = neobot.bot.call_api(action, params?)
```

返回值 `result` 为 table：`result.status` / `result.retcode` / `result.msg` / `result.data`（JSON string）。调用失败时 `result` 为 nil，`err` 为错误信息。

### 消息发送

| 方法 | 签名 | 说明 |
|---|---|---|
| `send_private_msg` | `(user_id, message)` | 发送私聊消息，返回 message_id |
| `send_group_msg` | `(group_id, message)` | 发送群消息，返回 message_id |
| `send_like` | `(user_id, times?)` | 点赞 |
| `delete_msg` | `(message_id)` | 撤回消息 |

`message` 参数可以是字符串（纯文本）或消息段数组。

### 群组操作

| 方法 | 签名 | 说明 |
|---|---|---|
| `get_group_list` | `()` | 返回 table 数组 |
| `get_group_info` | `(group_id)` | 返回 JSON string |
| `get_group_member_list` | `(group_id)` | 返回 JSON string |
| `get_group_member_info` | `(group_id, user_id)` | 返回 JSON string |
| `group_kick` | `(group_id, user_id, reject?)` | 踢出成员 |
| `group_ban` | `(group_id, user_id, duration?)` | 禁言（默认 1800 秒） |
| `set_group_card` | `(group_id, user_id, card)` | 设置群名片 |
| `set_group_whole_ban` | `(group_id, enable)` | 全员禁言 |

### 账号/好友

| 方法 | 签名 | 说明 |
|---|---|---|
| `get_login_info` | `()` | 返回 JSON string |
| `self_id` | `()` | 自身 QQ 号 |
| `get_stranger_info` | `(user_id)` | 返回 JSON string |
| `get_friend_list` | `()` | 返回 JSON string |

### 媒体

| 方法 | 签名 | 说明 |
|---|---|---|
| `can_send_image` | `()` | 能否发图片 |
| `can_send_record` | `()` | 能否发语音 |
| `get_image` | `(file)` | 获取图片信息 |

## neobot.seg

消息段构造器，每个方法返回标准 OneBot 消息段 table。

| 方法 | 签名 | 返回结构 |
|---|---|---|
| `seg.text(text)` | `(string)` | `{type="text", data={text=...}}` |
| `seg.image(file)` | `(string)` | `{type="image", data={file=...}}` |
| `seg.at(qq)` | `(number)` | `{type="at", data={qq=...}}` |
| `seg.face(id)` | `(number)` | `{type="face", data={id=...}}` |
| `seg.reply(id)` | `(number)` | `{type="reply", data={id=...}}` |
| `seg.record(file)` | `(string)` | `{type="record", data={file=...}}` |
| `seg.video(file)` | `(string)` | `{type="video", data={file=...}}` |
| `seg.json(data)` | `(string)` | `{type="json", data={data=...}}` |
| `seg.node(user_id, nickname, content)` | `(number, string, any)` | `{type="node", data={...}}` |

```lua
-- 组合发送
neobot.bot.send_group_msg(group_id, {
    neobot.seg.reply(message_id),
    neobot.seg.text("回复内容"),
    neobot.seg.at(user_id),
    neobot.seg.image("https://example.com/pic.png")
})
```

## neobot.event

事件上下文，仅在 handler 执行期间有效。

| 方法 | 返回类型 | 说明 |
|---|---|---|
| `event.user_id()` | number | 发送者 QQ |
| `event.group_id()` | number | 群号（私聊为 0） |
| `event.message_type()` | string | `"private"` / `"group"` |
| `event.raw_message()` | string | 原始消息文本 |
| `event.message_id()` | number | 消息 ID |
| `event.self_id()` | number | 自身 QQ |
| `event.segments()` | table | 当前消息的消息段数组 |

## neobot.util

| 方法 | 签名 | 说明 |
|---|---|---|
| `util.now()` | `() → number` | Unix 时间戳（秒） |
| `util.date(format?)` | `(string) → string` | 时间格式化 |
| `util.sleep(ms)` | `(number)` | 休眠（毫秒） |
| `util.http_get(url)` | `(string) → string, err` | HTTP GET |
| `util.http_post(url, body?, content_type?)` | `(string, string, string) → string, err` | HTTP POST |
| `util.base64_encode(str)` | `(string) → string` | Base64 编码 |
| `util.base64_decode(str)` | `(string) → string, err` | Base64 解码 |
| `util.md5(str)` | `(string) → string` | MD5 哈希 |
| `util.sha256(str)` | `(string) → string` | SHA256 哈希 |
| `util.json_encode(tbl)` | `(table) → string, err` | JSON 编码 |
| `util.json_decode(str)` | `(string) → table, err` | JSON 解码 |

日期格式化字符：`%Y` `%m` `%d` `%H` `%M` `%S` `%A` `%B` 等。

## neobot.perm

| 方法 | 签名 | 说明 |
|---|---|---|
| `perm.check(user_id, group_id, role, required_level)` | `(number, number, string, string) → bool` | 权限检查 |
| `perm.is_super(user_id)` | `(number) → bool` | 是否超级用户 |

## neobot.redis

仅在配置 Redis 后可用。检查 `redis.available()` 判断。

| 方法 | 签名 | 说明 |
|---|---|---|
| `redis.available()` | `() → bool` | 是否可用 |
| `redis.get(key)` | `(string) → string, err` | 获取值 |
| `redis.set(key, value, ttl?)` | `(string, string, number) → nil, err` | 设置值 |
| `redis.del(key, ...)` | `(string...) → nil, err` | 删除键 |
| `redis.exists(key)` | `(string) → bool, err` | 是否存在 |
| `redis.incr(key)` | `(string) → number, err` | 自增 |
| `redis.hget(key, field)` | `(string, string) → string, err` | Hash 读取 |
| `redis.hset(key, fields_table)` | `(string, table) → nil, err` | Hash 设置 |
| `redis.hgetall(key)` | `(string) → table, err` | Hash 全部读取 |
| `redis.lpush(key, val, ...)` | `(string, any...) → nil, err` | 列表左推 |
| `redis.rpush(key, val, ...)` | `(string, any...) → nil, err` | 列表右推 |
| `redis.lpop(key)` | `(string) → string, err` | 列表左弹出 |
| `redis.lrange(key, start, stop)` | `(string, number, number) → table, err` | 列表范围 |
| `redis.llen(key)` | `(string) → number, err` | 列表长度 |

## neobot.mysql

仅在配置 MySQL 后可用。

| 方法 | 签名 | 说明 |
|---|---|---|
| `mysql.available()` | `() → bool` | 是否可用 |
| `mysql.query(sql, args...)` | `(string, any...) → table, err` | 查询多条 |
| `mysql.query_one(sql, args...)` | `(string, any...) → table, err` | 查询单条 |
| `mysql.exec(sql, args...)` | `(string, any...) → number, err` | 执行修改（返回影响行数） |

SQL 参数使用 `?` 占位符。

## neobot.render

仅在配置浏览器渲染后可用。

| 方法 | 签名 | 说明 |
|---|---|---|
| `render.available()` | `() → bool` | 是否可用 |
| `render.html(html, width?)` | `(string, number) → string, err` | HTML 转图片（base64） |
| `render.url(url, width?)` | `(string, number) → string, err` | 网页截图 |
| `render.template(tpl, data, width?)` | `(string, table, number) → string, err` | 模板渲染 |

- `width` 默认：html/template 为 800，url 为 1280
- 返回值格式：`"base64://..."`，可直接传给 `seg.image()`

## 完整示例

```lua
-- plugin.lua
local nb = neobot

-- 注册命令
nb.register.command("mycmd", "user", function(args)
    local name = args[1] or "世界"
    return nb.config.get("greeting", "你好") .. ", " .. name .. "!"
end, { aliases = { "mc" } })

-- 消息 Hook
nb.register.on_message(function(text)
    if string.find(text, "你好") then
        return "你也好呀！"
    end
end)

-- 通知 Hook
nb.register.on_notice("group_increase", function(t)
    return "欢迎新成员！"
end)
```

## 依赖管理

```toml
[dependencies]
lua = ["string-utils"]   # 校验 lib/string-utils.lua 存在
local = ["./lib"]        # 设置 package.path
```

在代码中加载本地库：

```lua
package.path = neobot.package_paths .. ";./?.lua;" .. (package.path or "")
local str = require("string-utils")
```

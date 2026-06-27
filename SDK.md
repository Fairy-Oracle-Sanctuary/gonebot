# NeoBot SDK 文档

NeoBot Go 是多运行时 QQ 机器人框架的 SDK 文档，涵盖 Lua 和 Python 两种插件运行时的完整 API 参考、架构说明、通信协议、插件生命周期及最佳实践。

## 目录

- [架构概览](#架构概览)
- [通信协议](#通信协议)
- [插件生命周期](#插件生命周期)
- [插件元信息 (plugin.toml)](#插件元信息-plugintoml)
- [Lua SDK](#lua-sdk)
- [Python SDK](#python-sdk)
- [事件模型](#事件模型)
- [消息段规范](#消息段规范)
- [权限模型](#权限模型)
- [依赖管理](#依赖管理)
- [最佳实践](#最佳实践)

---

## 架构概览

```
┌─────────────────────────────────────────────────────────┐
│                    NeoBot Go (主进程)                       │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌───────────────┐         │
│  │ WS Client│  │  Bot     │  │  Permission   │         │
│  │ (NapCat) │  │ (API聚合)│  │  (三级权限)    │         │
│  └────┬─────┘  └────┬─────┘  └───────┬───────┘         │
│       │             │               │                   │
│  ┌────┴─────────────┴───────────────┴────────────────┐  │
│  │                   Router (事件分发)                  │  │
│  │  Message → MessageHooks → CommandLookup → Handler │  │
│  │  Notice  → NoticeHooks                            │  │
│  └─────────────┬─────────────────────────────────────┘  │
│                │                                        │
│  ┌─────────────┴─────────────────────────────────────┐  │
│  │             Plugin Manager                        │  │
│  │  ┌──────────────┐  ┌──────────────────────┐      │  │
│  │  │ Lua Runtime  │  │  Python Runtime       │      │  │
│  │  │ (嵌入式 VM)   │  │  (独立子进程)          │      │  │
│  │  │              │  │  JSON-RPC over stdio  │      │  │
│  │  └──────────────┘  └──────────────────────┘      │  │
│  └──────────────────────────────────────────────────┘  │
│                                                         │
│  可选服务: Redis │ MySQL │ Browser(渲染)                   │
└─────────────────────────────────────────────────────────┘
```

### Lua 运行时

- 每个插件一个独立的 `gopher-lua` VM 实例 ([lua.go](file:///c:/Users/yello/Documents/gonebot/internal/plugin/runtime/lua.go))
- SDK 以全局变量 `neobot` 注入到 VM 中 ([lua_sdk.go](file:///c:/Users/yello/Documents/gonebot/internal/plugin/runtime/lua_sdk.go))
- 注册的命令和 Hook 通过 Go 闭包桥接到 Registry
- 支持本地 Lua 库引用（通过 `[dependencies].local`）

### Python 运行时

- 每个插件一个独立的 Python 子进程 ([pythonproc/proc.go](file:///c:/Users/yello/Documents/gonebot/internal/plugin/pythonproc/proc.go))
- 通信协议: JSON-RPC over stdio
- Host 脚本: [pyplugin_host.py](file:///c:/Users/yello/Documents/gonebot/shim/pyplugin_host.py)
- SDK 包: `neobot_sdk` ([__init__.py](file:///c:/Users/yello/Documents/gonebot/shim/neobot_sdk/__init__.py), [plugin.py](file:///c:/Users/yello/Documents/gonebot/shim/neobot_sdk/plugin.py))
- 支持 venv 虚拟环境隔离

---

## 通信协议

### Lua: 直接调用

Lua 插件通过 Go 闭包直接在 Go 内存空间中被调用，无序列化开销。

### Python: JSON-RPC over stdio

Python 子进程与 Go 主进程通过 stdin/stdout 进行 JSON-RPC 通信：

```
Go → Python (事件分发):
  {"method":"event","id":"req_xxx","params":{...}}

Python → Go (API 调用):
  {"method":"call_api","id":"py_xxx","params":{"action":"send_private_msg","params":{...}}}

Go → Python (API 返回值):
  {"method":"bot_reply","id":"py_xxx","params":{...}}

Python → Go (事件回复):
  {"method":"event_reply","id":"req_xxx","params":{...}}

Go → Python (心跳):
  {"method":"ping"}

Python → Go:
  {"method":"pong"}

Go → Python (关闭):
  {"method":"shutdown"}
```

#### 事件分发消息格式

**命令事件：**
```json
{
  "method": "event",
  "id": "req_abc123",
  "params": {
    "event": "command",
    "cmd": "echo",
    "args": ["参数1", "参数2"],
    "plugin": "myplugin",
    "event_ctx": {
      "user_id": 123456,
      "group_id": 789012,
      "message_type": "group",
      "raw_message": "/echo 参数1 参数2",
      "message_id": 5555,
      "self_id": 111111,
      "segments": [
        {"type": "text", "data": {"text": "/echo"}},
        {"type": "text", "data": {"text": " 参数1 参数2"}}
      ]
    }
  }
}
```

**消息事件：**
```json
{
  "method": "event",
  "params": {
    "event": "message",
    "text": "消息纯文本内容",
    "event_ctx": { ... }
  }
}
```

**通知事件：**
```json
{
  "method": "event",
  "params": {
    "event": "notice",
    "noticeType": "group_increase",
    "event_ctx": { ... }
  }
}
```

---

## 插件生命周期

```
启动阶段:
  1. Manager.LoadAll() 扫描插件目录
  2. LoadMetadata() 读取 plugin.toml
  3. 根据 runtime 字段创建对应的 Runtime
  4. Runtime.Load() 启动插件

Lua 加载:
  a. 创建 lua.LState (新 VM)
  b. injectSDK() 注入 neobot 全局表
  c. L.DoFile() 执行 plugin.lua
  d. plugin.lua 中调用 neobot.register.* 完成注册

Python 加载:
  a. 检查并安装 [dependencies].python
  b. 创建 venv (如果启用)
  c. 启动子进程: python pyplugin_host.py --plugin=<name> --plugin-dir=<dir>
  d. 子进程加载 plugin.py，扫描 @command/@on_message/@on_notice
  e. 子进程发送 {"method":"ready", ...} 确认就绪
  f. Go 端收到 ready 后注册命令和 Hook

运行时:
  - 事件到达 → Router.Dispatch() → 匹配命令/Hook → 调用 handler
  - Go 端 Router 持有 Host.EventCtx (当前事件上下文)

热重载:
  - fsnotify 监听文件变更 (Write/Create/Remove)
  - 500ms 防抖
  - Manager.Reload(name) → Unregister → Runtime.Unload → Runtime.Load

关闭:
  a. Manager.Close() → 各 Runtime.Close()
  b. Lua: L.Close() 关闭 VM
  c. Python: 发送 shutdown → 等待退出 → KILL
```

---

## 插件元信息 (plugin.toml)

每个插件必须包含 `plugin.toml`，定义在 [plugin.go](file:///c:/Users/yello/Documents/gonebot/internal/plugin/plugin.go) 的 `Metadata` 结构体中。

```toml
# ---- 必填字段 ----
name = "myplugin"           # 插件唯一名称
runtime = "lua"            # 运行时: "lua" | "python"

# ---- 基本信息 (推荐填写) ----
version = "1.0.0"          # 语义化版本
author = "作者名"           # 作者
description = "插件描述"    # 简短描述
usage = "/mycmd <参数>"     # 使用说明

# ---- 权限 (可选, 默认 user) ----
permission = "user"         # user | admin | superuser
tags = ["工具"]             # 标签列表

# ---- 插件私有配置 ----
[config]
key1 = "value1"
key2 = 123

# ---- 依赖声明 ----
[dependencies]
python = ["requests>=2.31"]   # pip 包
lua = ["string-utils"]        # Lua 库
local = ["./lib"]             # 本地路径
```

### Metadata 结构体 (Go 侧)

```go
type Metadata struct {
    Name         string           `toml:"name"`
    Version      string           `toml:"version"`
    Author       string           `toml:"author"`
    Description  string           `toml:"description"`
    Usage        string           `toml:"usage"`
    Permission   string           `toml:"permission"`
    Tags         []string         `toml:"tags"`
    Runtime      string           `toml:"runtime"`
    Config       map[string]any   `toml:"config"`
    Dependencies *Dependencies    `toml:"dependencies,omitempty"`
}

type Dependencies struct {
    Python []string `toml:"python,omitempty"`
    Lua    []string `toml:"lua,omitempty"`
    Local  []string `toml:"local,omitempty"`
}
```

---

## Lua SDK

Lua SDK 通过全局变量 `neobot` 注入到每个插件的 VM 中。SDK 注入逻辑在 [lua_sdk.go](file:///c:/Users/yello/Documents/gonebot/internal/plugin/runtime/lua_sdk.go)。

### 顶层模块

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
| `neobot.redis` | Redis 操作 (可选) |
| `neobot.mysql` | MySQL 操作 (可选) |
| `neobot.render` | 图片渲染 (可选) |

---

### neobot.log

```lua
neobot.log.debug(msg)
neobot.log.info(msg)
neobot.log.warn(msg)
neobot.log.error(msg)
```

- `msg`: 字符串，日志内容
- 输出带有 `[plugin=<name>]` 和 `[runtime=lua]` 标记

### neobot.config

```lua
local value = neobot.config.get(key, default_value)
```

| 参数 | 类型 | 说明 |
|---|---|---|
| `key` | string | 配置键名 |
| `default_value` | any | 默认值 (键不存在时返回) |

读取 `plugin.toml` 中 `[config]` 段的值。支持 string、number、boolean 类型的返回值。

### neobot.register

#### register.command

```lua
neobot.register.command(name, permission, handler, options?)
```

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 命令名（不含前缀 `/`） |
| `permission` | string | 是 | `"user"` / `"admin"` / `"superuser"` |
| `handler` | function | 是 | `function(args) end`，`args` 是字符串数组 |
| `options` | table | 否 | `{ aliases = {"别名1", "别名2"} }` |

**返回值：** handler 返回字符串则自动回复，返回 nil 则不回复。

```lua
neobot.register.command("echo", "user", function(args)
    if #args == 0 then
        return "用法: /echo <内容>"
    end
    return table.concat(args, " ")
end, { aliases = { "e", "复读" } })
```

#### register.on_message

```lua
neobot.register.on_message(handler)
```

| 参数 | 类型 | 说明 |
|---|---|---|
| `handler` | function | `function(text) end`，`text` 是消息纯文本 |

每条消息都会触发所有插件的消息 Hook。第一个返回非空字符串的 Hook 会触发自动回复并停止后续 Hook。

#### register.on_notice

```lua
neobot.register.on_notice(notice_type, handler)
```

| 参数 | 类型 | 说明 |
|---|---|---|
| `notice_type` | string | 通知类型（见下方） |
| `handler` | function | `function(notice_type_string) end` |

**支持的通知类型：**

| 常量 | 说明 | 分发来源 |
|---|---|---|
| `group_upload` | 群文件上传 | OneBot notice_type |
| `group_admin` | 群管理员变更 | OneBot notice_type |
| `group_decrease` | 群成员减少(退群/被踢) | OneBot notice_type |
| `group_increase` | 群成员增加(入群) | OneBot notice_type |
| `group_ban` | 群禁言 | OneBot notice_type |
| `friend_add` | 好友添加 | OneBot notice_type |
| `group_recall` | 群消息撤回 | OneBot notice_type |
| `friend_recall` | 私聊消息撤回 | OneBot notice_type |
| `notify` | 其他通知(戳一戳/红包等) | OneBot notice_type |

### neobot.bot

所有 Bot API 通过 [bot.go](file:///c:/Users/yello/Documents/gonebot/internal/bot/bot.go) 实现。

#### 通用 API 调用

```lua
local result, err = neobot.bot.call_api(action, params?)
```

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `action` | string | 是 | OneBot API action 名称 |
| `params` | table | 否 | API 参数字典 |

**返回值：** `result` 为 table，包含：
- `result.status` — "ok" / "failed"
- `result.retcode` — 返回码 (0 = 成功)
- `result.msg` — 消息字符串
- `result.data` — JSON 字符串

调用失败时 `result` 为 nil，`err` 为错误信息字符串。

#### 消息发送

| 方法 | 签名 | 返回值 | 说明 |
|---|---|---|---|
| `send_private_msg` | `(user_id, message)` | `message_id` 或 `nil, err` | 发送私聊消息 |
| `send_group_msg` | `(group_id, message)` | `message_id` 或 `nil, err` | 发送群消息 |
| `send_like` | `(user_id, times?)` | `true` 或 `nil, err` | 点赞 |
| `delete_msg` | `(message_id)` | `true` 或 `nil, err` | 撤回消息 |

`message` 参数接受：
- **字符串** — 纯文本消息
- **Table (消息段数组)** — 组合消息，如 `{seg.text("hello"), seg.image("url")}`

#### 群组操作

| 方法 | 签名 | 返回值 | 说明 |
|---|---|---|---|
| `get_group_list` | `()` | table 数组 或 `nil, err` | 群列表 |
| `get_group_info` | `(group_id)` | JSON string 或 `nil, err` | 群信息 |
| `get_group_member_list` | `(group_id)` | JSON string 或 `nil, err` | 群成员列表 |
| `get_group_member_info` | `(group_id, user_id)` | JSON string 或 `nil, err` | 成员信息 |
| `group_kick` | `(group_id, user_id, reject?)` | `true` 或 `nil, err` | 踢出成员 |
| `group_ban` | `(group_id, user_id, duration?)` | `true` 或 `nil, err` | 禁言 (默认1800秒) |
| `set_group_card` | `(group_id, user_id, card)` | `true` 或 `nil, err` | 设置群名片 |
| `set_group_whole_ban` | `(group_id, enable)` | `true` 或 `nil, err` | 全员禁言 |

#### 账号/好友

| 方法 | 签名 | 返回值 | 说明 |
|---|---|---|---|
| `get_login_info` | `()` | JSON string 或 `nil, err` | 登录信息 |
| `self_id` | `()` | number | 自身 QQ 号 |
| `get_stranger_info` | `(user_id)` | JSON string 或 `nil, err` | 陌生人信息 |
| `get_friend_list` | `()` | JSON string 或 `nil, err` | 好友列表 |

#### 媒体

| 方法 | 签名 | 返回值 | 说明 |
|---|---|---|---|
| `can_send_image` | `()` | JSON string 或 `nil, err` | 能否发图片 |
| `can_send_record` | `()` | JSON string 或 `nil, err` | 能否发语音 |
| `get_image` | `(file)` | JSON string 或 `nil, err` | 获取图片信息 |

### neobot.seg

消息段构造器，内部实现为 `buildSegT()` (lua_sdk.go:776)。每个方法返回一个标准 OneBot 消息段 table。

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
| `seg.node(user_id, nickname, content)` | `(number, string, any)` | `{type="node", data={user_id=..., nickname=..., content=...}}` |

### neobot.event

事件上下文，实现为 `buildEventT()` (lua_sdk.go:868)。仅在 handler 执行期间有效。

| 方法 | 返回类型 | 说明 |
|---|---|---|
| `event.user_id()` | number | 发送者 QQ |
| `event.group_id()` | number | 群号（私聊为 0） |
| `event.message_type()` | string | `"private"` / `"group"` |
| `event.raw_message()` | string | 原始消息文本 |
| `event.message_id()` | number | 消息 ID |
| `event.self_id()` | number | 自身 QQ |
| `event.segments()` | table | 当前消息的消息段数组 |

### neobot.util

工具函数集合。

| 方法 | 签名 | 说明 |
|---|---|---|
| `util.now()` | `() → number` | Unix 时间戳（秒） |
| `util.date(format?)` | `(string) → string` | 时间格式化（%Y %m %d %H %M %S） |
| `util.sleep(ms)` | `(number)` | 休眠（毫秒） |
| `util.http_get(url)` | `(string) → string, err` | HTTP GET |
| `util.http_post(url, body?, content_type?)` | `(string, string, string) → string, err` | HTTP POST（默认 JSON） |
| `util.base64_encode(str)` | `(string) → string` | Base64 编码 |
| `util.base64_decode(str)` | `(string) → string, err` | Base64 解码 |
| `util.md5(str)` | `(string) → string` | MD5 哈希 (hex) |
| `util.sha256(str)` | `(string) → string` | SHA256 哈希 (hex) |
| `util.json_encode(tbl)` | `(table) → string, err` | JSON 编码 |
| `util.json_decode(str)` | `(string) → table, err` | JSON 解码 |

**日期格式化字符：**

| 符 | 含义 | 示例 |
|---|---|---|
| `%Y` | 四位年份 | 2026 |
| `%y` | 两位年份 | 26 |
| `%m` | 月份 | 01-12 |
| `%d` | 日 | 01-31 |
| `%H` | 小时(24h) | 00-23 |
| `%I` | 小时(12h) | 01-12 |
| `%M` | 分钟 | 00-59 |
| `%S` | 秒 | 00-59 |
| `%p` | AM/PM | AM |
| `%A` | 星期全称 | Monday |
| `%a` | 星期简写 | Mon |
| `%B` | 月份全称 | January |
| `%b` | 月份简写 | Jan |

### neobot.perm

权限检查接口。

| 方法 | 签名 | 说明 |
|---|---|---|
| `perm.check(user_id, group_id, role, required_level)` | `(number, number, string, string) → bool` | 检查用户是否满足权限 |
| `perm.is_super(user_id)` | `(number) → bool` | 检查是否为超级用户 |

- `role`: OneBot 返回的 `sender.role`（`"owner"` / `"admin"` / `"member"`）
- `required_level`: `"user"` / `"admin"` / `"superuser"`

### neobot.redis

Redis 操作接口，仅在配置 Redis 连接后可用。实现为 `buildRedisT()` (lua_sdk.go:503)。

| 方法 | 签名 | 说明 |
|---|---|---|
| `redis.available()` | `() → bool` | 检查是否可用 |
| `redis.get(key)` | `(string) → string, err` | 获取值 |
| `redis.set(key, value, ttl?)` | `(string, string, number) → nil, err` | 设置值（TTL 秒） |
| `redis.del(key, ...)` | `(string...) → nil, err` | 删除键 |
| `redis.exists(key)` | `(string) → bool, err` | 是否存在 |
| `redis.incr(key)` | `(string) → number, err` | 自增 |
| `redis.hget(key, field)` | `(string, string) → string, err` | Hash 读取 |
| `redis.hset(key, fields_table)` | `(string, table) → nil, err` | Hash 设置 |
| `redis.hgetall(key)` | `(string) → table, err` | Hash 全部读取 |
| `redis.lpush(key, val, ...)` | `(string, any...) → nil, err` | 列表左推 |
| `redis.rpush(key, val, ...)` | `(string, any...) → nil, err` | 列表右推 |
| `redis.lpop(key)` | `(string) → string, err` | 列表左弹出 |
| `redis.lrange(key, start, stop)` | `(string, number, number) → table, err` | 列表范围读取 |
| `redis.llen(key)` | `(string) → number, err` | 列表长度 |

### neobot.mysql

MySQL 操作接口，仅在配置 MySQL DSN 后可用。实现为 `buildMySQLT()` (lua_sdk.go:638)。

| 方法 | 签名 | 说明 |
|---|---|---|
| `mysql.available()` | `() → bool` | 检查是否可用 |
| `mysql.query(sql, args...)` | `(string, any...) → table, err` | 查询多条 |
| `mysql.query_one(sql, args...)` | `(string, any...) → table, err` | 查询单条 |
| `mysql.exec(sql, args...)` | `(string, any...) → number, err` | 执行修改（返回影响行数） |

SQL 参数使用 `?` 占位符。

### neobot.render

图片渲染接口，仅在配置浏览器渲染后可用。实现为 `buildRenderT()` (lua_sdk.go:713)。

| 方法 | 签名 | 说明 |
|---|---|---|
| `render.available()` | `() → bool` | 检查是否可用 |
| `render.html(html, width?)` | `(string, number) → string, err` | HTML 转 base64 图片 |
| `render.url(url, width?)` | `(string, number) → string, err` | 网页截图 |
| `render.template(tpl, data, width?)` | `(string, table, number) → string, err` | Go template 渲染 |

- `width` 默认值：`html`/`template` 为 800，`url` 为 1280
- 返回值格式：`"base64://..."` ，可直接传给 `seg.image()`

---

## Python SDK

Python SDK 包 `neobot_sdk` 位于 [shim/neobot_sdk/](file:///c:/Users/yello/Documents/gonebot/shim/neobot_sdk/)。SDK 对象由 Host 在每次事件分发前注入到 handler 调用中。

### 装饰器

```python
from neobot_sdk import command, on_message, on_notice
```

#### @command

```python
@command(name: str | None = None, permission: str = "user", aliases: List[str] | None = None)
```

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `name` | str \| None | 函数名 | 命令名 |
| `permission` | str | `"user"` | 权限级别 |
| `aliases` | List[str] \| None | `None` | 别名列表 |

被装饰的方法签名：

```python
async def handler(self, sdk, params: dict) -> str | None:
    args = params.get("args", [])   # List[str]
    sdk  # SDK 实例
    return "回复文本"  # 返回 None 不回复
```

#### @on_message

```python
@on_message
```

被装饰的方法签名：

```python
async def handler(self, sdk, params: dict) -> str | None:
    text = params.get("text", "")   # str: 消息纯文本
    return "回复"  # 返回 None 不回复
```

#### @on_notice

```python
@on_notice
```

被装饰的方法签名：

```python
async def handler(self, sdk, params: dict) -> None:
    notice_type = params.get("noticeType", "")  # str: 通知类型
```

### Plugin 基类

```python
from neobot_sdk import Plugin
```

```python
class Plugin:
    name: str = ""            # 插件名
    version: str = "1.0.0"    # 版本
    description: str = ""     # 描述

    async def on_init(self) -> None:     # 初始化钩子 (可选)
        ...

    async def on_shutdown(self) -> None:  # 关闭钩子 (可选)
        ...

    def get_meta(self) -> dict:          # 获取元信息
        return {"name": ..., "version": ..., "description": ...}
```

### sdk 对象

在 handler 中通过 `sdk` 参数访问。SDK 类定义在 [pyplugin_host.py:307](file:///c:/Users/yello/Documents/gonebot/shim/pyplugin_host.py#L307)。

#### sdk.bot — Bot API

Bot 类定义在 [pyplugin_host.py:204](file:///c:/Users/yello/Documents/gonebot/shim/pyplugin_host.py#L204)，通过 JSON-RPC `call_api` 调用 Go 端。

| 方法 | 签名 | 返回值 | 说明 |
|---|---|---|---|
| `call_api` | `(action, params?)` | any | 通用 OneBot API 调用 |
| `send_private_msg` | `(user_id, message)` | dict | 发送私聊消息 |
| `send_group_msg` | `(group_id, message)` | dict | 发送群消息 |
| `send_msg` | `(*, user_id=0, group_id=0, message)` | dict | 自动判断发送方式 |
| `delete_msg` | `(message_id)` | dict | 撤回消息 |
| `get_msg` | `(message_id)` | dict | 获取消息详情 |
| `send_like` | `(user_id, times=1)` | dict | 点赞 |
| `get_group_list` | `()` | List[dict] | 群列表 |
| `get_group_info` | `(group_id)` | dict | 群信息 |
| `get_group_member_list` | `(group_id)` | List[dict] | 群成员列表 |
| `get_group_member_info` | `(group_id, user_id)` | dict | 群成员信息 |
| `group_kick` | `(group_id, user_id, reject_add_request=False)` | dict | 踢出成员 |
| `group_ban` | `(group_id, user_id, duration=1800)` | dict | 禁言 |
| `set_group_card` | `(group_id, user_id, card="")` | dict | 设置群名片 |
| `set_group_whole_ban` | `(group_id, enable=True)` | dict | 全员禁言 |
| `set_group_name` | `(group_id, group_name)` | dict | 设置群名 |
| `get_stranger_info` | `(user_id)` | dict | 陌生人信息 |
| `get_friend_list` | `()` | List[dict] | 好友列表 |
| `get_login_info` | `()` | dict | 登录信息 |
| `can_send_image` | `()` | dict | 能否发送图片 |
| `can_send_record` | `()` | dict | 能否发送语音 |
| `get_image` | `(file)` | dict | 获取图片信息 |

#### sdk.seg — 消息段构造器

MessageSegment 类定义在 [pyplugin_host.py:164](file:///c:/Users/yello/Documents/gonebot/shim/pyplugin_host.py#L164)。

| 方法 | 签名 | 返回类型 |
|---|---|---|
| `seg.text(text)` | `(str)` | OneBot 消息段 dict |
| `seg.image(file)` | `(str)` | OneBot 消息段 dict |
| `seg.at(qq)` | `(int)` | OneBot 消息段 dict |
| `seg.face(id_)` | `(int)` | OneBot 消息段 dict |
| `seg.reply(msg_id)` | `(int)` | OneBot 消息段 dict |
| `seg.record(file)` | `(str)` | OneBot 消息段 dict |
| `seg.video(file)` | `(str)` | OneBot 消息段 dict |
| `seg.json(data)` | `(str)` | OneBot 消息段 dict |
| `seg.node(user_id, nickname, content)` | `(int, str, any)` | OneBot 消息段 dict |

#### sdk.event — 事件上下文

Event 类定义在 [pyplugin_host.py:285](file:///c:/Users/yello/Documents/gonebot/shim/pyplugin_host.py#L285)。

| 属性 | 类型 | 说明 |
|---|---|---|
| `sdk.event.user_id` | int | 发送者 QQ |
| `sdk.event.group_id` | int | 群号（私聊为 0） |
| `sdk.event.message_type` | str | `"private"` / `"group"` |
| `sdk.event.raw_message` | str | 原始消息文本 |
| `sdk.event.message_id` | int | 消息 ID |
| `sdk.event.self_id` | int | 自身 QQ |
| `sdk.event.segments` | List[dict] | 消息段数组 |

#### sdk.log — 日志

```python
sdk.log.debug("调试信息")
sdk.log.info("普通信息")
sdk.log.warning("警告信息")
sdk.log.error("错误信息")
```

使用 Python 标准 `logging`，输出到 stderr，被 Go 端捕获并转发到主日志系统。

---

## 事件模型

OneBot v11 事件模型定义在 [event.go](file:///c:/Users/yello/Documents/gonebot/internal/event/event.go)。

### 事件类型

```
PostMessage (message)  ─→ MessageEvent
PostNotice  (notice)   ─→ NoticeEvent
PostRequest (request)  ─→ RequestEvent
PostMeta    (meta_event) → MetaEvent
```

### MessageEvent

```go
type MessageEvent struct {
    Event                               // time, self_id, post_type
    SubType     MessageSubType          // friend / normal / anonymous
    MessageID   int64
    UserID      int64
    MessageType MessageType             // private / group
    GroupID     int64
    Message     []MessageSegment
    RawMessage  string
    Font        int
    Sender      Sender                  // user_id, nickname, card, sex, age, area, role, title
    Anonymous   *Anonymous
}
```

### NoticeEvent

```go
type NoticeEvent struct {
    Event
    NoticeType NoticeType   // group_upload / group_admin / group_decrease / group_increase / group_ban / friend_add / group_recall / friend_recall / notify
    GroupID    int64
    UserID     int64
    OperatorID int64
    SubType    string
    File       *File
    MessageID  int64
    TargetID   int64
    Duration   int
}
```

### Sender 结构

```go
type Sender struct {
    UserID   int64
    Nickname string
    Card     string   // 群名片
    Sex      string
    Age      int
    Area     string
    Role     string   // owner / admin / member
    Title    string
}
```

---

## 消息段规范

消息段是 OneBot v11 的标准消息格式，一个消息由多个消息段组成的数组表示。

### 消息段类型

| 类型 | type 字符串 | data 字段 | 说明 |
|---|---|---|---|
| 文本 | `text` | `{text: string}` | 纯文本 |
| 图片 | `image` | `{file: string}` | 图片文件/URL/base64 |
| @ | `at` | `{qq: string}` | @某人 |
| 表情 | `face` | `{id: string}` | QQ 表情ID |
| 引用 | `reply` | `{id: string}` | 引用回复 |
| 语音 | `record` | `{file: string}` | 语音文件 |
| 视频 | `video` | `{file: string}` | 视频文件 |
| JSON | `json` | `{data: string}` | JSON 卡片消息 |
| 合并转发 | `node` | `{user_id: string, nickname: string, content: any}` | 转发节点 |

### 使用示例

**Lua:**
```lua
neobot.bot.send_group_msg(group_id, {
    neobot.seg.reply(message_id),
    neobot.seg.text("回复内容"),
    neobot.seg.at(user_id),
    neobot.seg.image("https://example.com/pic.png")
})
```

**Python:**
```python
sdk.bot.send_group_msg(group_id, [
    sdk.seg.reply(message_id),
    sdk.seg.text("回复内容"),
    sdk.seg.at(user_id),
    sdk.seg.image("https://example.com/pic.png")
])
```

---

## 权限模型

三级权限模型定义在 [permission.go](file:///c:/Users/yello/Documents/gonebot/internal/permission/permission.go)。

```
superuser  (最高)
  └─ admin    (群主/管理员/管理员群)
       └─ user    (所有用户，默认)
```

### 权限检查逻辑

```
if required == user     → 直接通过
if user in superusers   → 直接通过
if required == superuser → 拒绝 (非超级用户)
if role is owner/admin  → 通过
if group in admin_groups → 通过
否则 → 拒绝
```

### 权限常量

```go
const (
    User      Level = 0  // "user"
    Admin     Level = 1  // "admin"
    SuperUser Level = 2  // "superuser"
)
```

### 使用

- **plugin.toml**: `permission = "admin"` — 插件所有命令的默认权限
- **命令注册**: `neobot.register.command("name", "superuser", handler)` — 命令级别覆盖
- **运行时检查**: `neobot.perm.check(user_id, group_id, role, "admin")`

---

## 依赖管理

依赖管理模块定义在 [deps/](file:///c:/Users/yello/Documents/gonebot/internal/plugin/deps/)。

### Python 依赖

```toml
[dependencies]
python = ["requests>=2.31", "pillow>=10.0"]
```

**安装流程：**
1. 创建/检查 venv（`plugins_py/.venv` 共享或插件独立）
2. 将依赖写入 `.nb_reqs.txt`
3. 计算依赖 hash，与已安装的比较
4. 不变则跳过；变化则执行 `pip install`
5. 优先使用 `uv pip install`（更快），回退到 `python -m pip`

**配置项：**
- `use_venv = true` — 使用虚拟环境隔离
- `shared_venv_path = "plugins_py/.venv"` — 多插件共享 venv
- `pip_index` — 自定义 PyPI 镜像源
- `pip_extra_args` — 额外 pip 参数

### Lua 依赖

```toml
[dependencies]
lua = ["string-utils"]   # 校验 lib/string-utils.lua 存在
local = ["./lib"]        # 设置 package.path
```

- `lua`: 校验 `插件目录/lib/<name>.lua` 文件存在，不存在则加载失败
- `local`: 在加载插件前自动拼接路径，可通过 `neobot.package_paths` 获取

加载本地库示例：
```lua
-- plugin.toml 中声明 [dependencies].local = ["./lib"]
-- 然后在插件中:
package.path = neobot.package_paths .. ";./?.lua;./?/init.lua;" .. (package.path or "")
local mylib = require("string-utils")
```

---

## 最佳实践

### 命令设计

1. 命令名使用小写英文 + 连字符，如 `my-cmd`
2. 提供中文别名方便用户使用
3. 在 `usage` 字段中写清楚参数格式
4. 无参数时提供帮助提示

### 消息 Hook

1. 避免过于宽泛的关键词匹配（如单个中文字符）
2. 对于高频触发的 Hook，考虑添加冷却（利用 `neobot.redis`）
3. 返回 `nil` 不回复，让后续 Hook 继续处理

### 错误处理

**Lua:**
```lua
local ok, err = pcall(function()
    -- 可能失败的操作
end)
if not ok then
    neobot.log.error("操作失败: " .. tostring(err))
    return "出错了, 请稍后重试"
end
```

**Python:**
```python
try:
    # 可能失败的操作
except Exception as e:
    sdk.log.error(f"操作失败: {e}")
    return "出错了, 请稍后重试"
```

### 性能

1. 命令 Handler 应快速返回，避免长时间阻塞
2. 耗时操作使用异步（Python: `async/await`; Lua: 无原生支持，避免阻塞）
3. 大量数据存储使用 Redis 而非内存
4. API 调用会阻塞当前事件处理，勿在 Handler 中做大量串行 API 调用

### 状态管理

1. 持久化数据使用 Redis 或 MySQL
2. 避免在 Lua/Python 全局变量中存储需要持久化的状态（热重载会丢失）
3. 插件私有配置放在 `plugin.toml` 的 `[config]` 段

### Python 插件注意事项

1. **子进程隔离**: 每个 Python 插件在独立进程中运行，无法共享内存状态
2. **同步调用**: `sdk.bot.call_api()` 是同步的（底层通过 threading.Event 阻塞等待 JSON-RPC 响应）
3. **不要在 handler 内主动调用 `sdk.bot.send_msg`**: 这会通过 call_api 发送请求给 Go，而 Go 正在等待当前事件处理完成，可能造成死锁。在 handler 中应该 `return` 回复内容而非主动发送
4. **依赖声明**: import 的第三方包必须在 `plugin.toml` 的 `[dependencies].python` 中声明
5. **venv 共享**: 多个插件可共享 venv（配置 `shared_venv_path`），减少磁盘占用

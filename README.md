# NeoBot Go

基于 OneBot v11 协议的多运行时 QQ 机器人框架。使用 Go 编写核心，支持 **Lua** 和 **Python** 两种插件运行时，提供热重载、依赖管理、图片渲染等完整能力。

## 特性

- **多运行时支持** — Lua 插件（嵌入式 VM）和 Python 插件（独立子进程）
- **OneBot v11 兼容** — 正向/反向 WebSocket 连接，兼容 NapCat、LLOneBot 等实现
- **命令系统** — 注册命令 + 别名 + 三级权限（user / admin / superuser）
- **事件 Hook** — 全局消息 hook、通知 hook（群成员增减、群文件上传等）
- **热重载** — 修改插件文件后 500ms 内自动重新加载
- **依赖管理** — Python 插件自动安装 pip 依赖（支持 venv 隔离）；Lua 插件支持本地库引用
- **服务集成** — 内置 Redis、MySQL、浏览器渲染（HTML 转图片）可选服务
- **图片渲染** — 浏览器池支持 HTML/URL/Template 转图片，直接作为消息发送

## 项目结构

```
gonebot/
├── cmd/neobot/main.go              # 入口
├── internal/
│   ├── bot/                        # Bot 聚合根 (OneBot API 封装)
│   ├── config/                     # TOML 配置加载
│   ├── event/                      # OneBot v11 事件模型
│   ├── initfiles/                  # 首次运行自动创建目录/示例
│   ├── logger/                     # slog 结构化日志
│   ├── logx/                       # 多语言日志
│   ├── permission/                 # 三级权限模型
│   ├── plugin/                     # 插件系统核心
│   │   ├── plugin.go               #   元数据解析
│   │   ├── registry.go             #   命令/事件注册表
│   │   ├── manager.go              #   插件管理器
│   │   ├── router.go               #   事件路由分发
│   │   ├── watcher.go              #   fsnotify 热重载
│   │   ├── deps/                   #   依赖安装 (pip/venv)
│   │   ├── pythonproc/             #   Python 子进程管理
│   │   └── runtime/
│   │       ├── runtime.go          #   Runtime 接口 + Host
│   │       ├── lua.go              #   Lua 运行时
│   │       ├── lua_sdk.go          #   Lua SDK 注入 (neobot.*)
│   │       ├── python.go           #   Python 运行时
│   │       └── metadata.go         #   插件元信息
│   ├── reversews/                  # 反向 WS 服务端
│   ├── service/
│   │   ├── browser/                #   Rod 浏览器池
│   │   ├── image/                  #   HTML 渲染
│   │   ├── mysql/                  #   MySQL 服务
│   │   └── redis/                  #   Redis 服务
│   └── ws/                         # WebSocket 客户端
├── shim/
│   ├── pyplugin_host.py            # Python 插件宿主 (JSON-RPC)
│   └── neobot_sdk/                 # Python SDK
│       ├── __init__.py
│       └── plugin.py
├── plugins_lua/                    # Lua 插件目录
│   ├── echo/                       #   复读机示例
│   ├── hello/                      #   基础示例
│   └── greeter/                    #   本地库引用示例
├── plugins_py/                     # Python 插件目录
│   ├── echo/                       #   复读机示例
│   └── webfetch/                   #   依赖安装示例 (requests)
├── config.toml.example             # 配置模板
├── go.mod
└── README.md
```

## 快速开始

### 前提条件

- **Go** 1.26+
- **Python** 3.10+（仅 Python 插件需要）
- 一个运行的 OneBot v11 实现（推荐 [NapCat](https://github.com/NapNeko/NapCatQQ)）

### 1. 编译

```bash
git clone <仓库地址> gonebot
cd gonebot
go build -o neobot.exe ./cmd/neobot
```

### 2. 配置

```bash
copy config.toml.example config.toml
```

编辑 `config.toml`，至少填写：

```toml
[bot]
self_id = 0           # 首次运行自动获取, 也可手动填写
superusers = [123456] # 超级用户 QQ 号列表
admin_groups = []     # 管理员群号列表

[napcat_ws]
uri = "ws://127.0.0.1:30001"  # NapCat 正向 WS 地址
token = ""                     # Access Token (与 NapCat 一致)

[plugins.lua]
enabled = true
dir = "plugins_lua"

[plugins.python]
enabled = false         # 如需 Python 插件请开启
dir = "plugins_py"
python_bin = "python3"
```

### 3. 启动 NapCat

确保 NapCat 已配置正向 WebSocket 服务并运行。

### 4. 启动 NeoBot

```bash
.\neobot.exe
```

首次运行会自动创建插件示例目录和默认配置文件。

## 配置参考

以下是 `config.toml` 的完整配置项说明：

### [bot] — Bot 基本设置

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `self_id` | int | `0` | Bot QQ 号（0 = 自动获取） |
| `nickname` | string | `"NEO Bot"` | Bot 昵称 |
| `command_prefix` | []string | `["/", "!", "＃"]` | 命令前缀（会自动去除） |
| `ignore_self_message` | bool | `true` | 是否忽略自己发的消息 |
| `superusers` | []int64 | `[]` | 超级用户 QQ 号列表 |
| `admin_groups` | []int64 | `[]` | 管理员群号列表 |

### [napcat_ws] — 正向 WebSocket 连接

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `uri` | string | `"ws://127.0.0.1:30001"` | OneBot WS 地址 |
| `token` | string | `""` | Access Token |
| `reconnect_interval` | int | `5` | 重连间隔(秒) |
| `heartbeat_interval` | int | `30` | 心跳间隔(秒) |
| `api_request_timeout` | int | `30` | API 请求超时(秒) |

### [reverse_ws] — 反向 WebSocket 服务端

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 是否启用 |
| `host` | string | `"0.0.0.0"` | 监听地址 |
| `port` | int | `8080` | 监听端口 |
| `token` | string | `""` | Access Token |

### [plugins.lua] — Lua 插件

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 是否启用 |
| `dir` | string | `"plugins_lua"` | 插件目录 |
| `max_memory_mb` | int | `64` | 最大内存(MB) |
| `max_cpu_sec` | int | `5` | 最大 CPU 时间(秒) |

### [plugins.python] — Python 插件

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 是否启用 |
| `dir` | string | `"plugins_py"` | 插件目录 |
| `python_bin` | string | `"python3"` | Python 可执行文件 |
| `shim_path` | string | `"shim/pyplugin_host.py"` | Shim 脚本路径 |

### [plugins.deps] — 依赖管理

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 是否启用自动依赖管理 |
| `python_bin` | string | `"python3"` | pip 使用的 Python |
| `pip_index` | string | `""` | pip 镜像源 (如 `https://pypi.tuna.tsinghua.edu.cn/simple/`) |
| `pip_extra_args` | []string | `[]` | pip 额外参数 |
| `cache_dir` | string | `"./data/plugin_deps"` | 依赖缓存目录 |
| `timeout_sec` | int | `300` | 安装超时(秒) |
| `use_venv` | bool | `true` | 是否使用 venv 隔离 |
| `shared_venv_path` | string | `"plugins_py/.venv"` | 共享 venv 路径 |

### [redis] / [mysql] — 可选服务

见 `config.toml.example`。Redis/MySQL 仅在配置 `addr`/`dsn` 后才会连接，未配置时 SDK 中对应模块不可用。

### [browser] / [image] — 浏览器渲染

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `browser.enabled` | bool | `false` | 是否启用浏览器渲染 |
| `browser.pool_size` | int | `3` | 浏览器实例池大小 |
| `browser.headless` | bool | `true` | 无头模式 |
| `image.width` | int | `800` | 默认渲染宽度 |
| `image.quality` | int | `90` | 默认渲染质量 |
| `image.timeout_sec` | int | `30` | 渲染超时(秒) |

## 权限系统

NeoBot 使用三级权限模型：

| 等级 | 说明 | 判定方式 |
|---|---|---|
| `user` | 普通用户 | 所有用户 |
| `admin` | 管理员 | 群主/群管理 或 在 `admin_groups` 中 |
| `superuser` | 超级用户 | 在 `superusers` 列表中 |

- 超级用户拥有所有权限
- 命令可在 `plugin.toml` 中通过 `permission` 字段声明最低权限要求
- 权限不足时自动回复"权限不足"

## 插件开发

每个插件是一个独立的目录，包含一个 `plugin.toml` 元信息文件和一个入口文件（`plugin.lua` 或 `plugin.py`）。

插件目录名以 `_` 或 `.` 开头的会被跳过，不会加载。

### plugin.toml 格式

```toml
name = "myplugin"           # 必填: 插件名称
version = "1.0.0"          # 版本号
author = "作者"             # 作者
description = "插件描述"    # 描述
usage = "/mycmd <参数>"     # 使用说明
permission = "user"         # 最低权限: user | admin | superuser
runtime = "lua"            # 运行时: lua | python
tags = ["工具", "娱乐"]     # 标签

[config]                   # 插件私有配置 (通过 SDK 读取)
key1 = "value1"
key2 = 123

[dependencies]             # 依赖声明
python = ["requests>=2.31"]   # Python pip 包
lua = ["string-utils"]        # Lua 库名 (校验 lib/<name>.lua 存在)
local = ["./lib"]             # 本地 Lua 路径 (设置 package.path)
```

### 事件处理流程

```
[OneBot WS 消息] 
  → Router.Dispatch()
    → 全局消息 Hook (所有插件)
    → 命令匹配 + 权限检查
    → 通知 Hook (群成员增/减/禁言等)
```

- **消息 Hook** 返回值非空则自动回复
- **命令 Handler** 返回值非空则自动回复
- 命令前缀 (`/`, `!`, `＃`) 会自动去除

---

# Lua SDK 参考

Lua 插件通过全局变量 `neobot` (别名 `nb`) 访问 SDK。

## neobot.log — 日志

```lua
neobot.log.debug("调试信息")
neobot.log.info("普通信息")
neobot.log.warn("警告信息")
neobot.log.error("错误信息")
```

日志会输出到 NeoBot 的日志系统中，带有 `[plugin=<name>]` 标记。

## neobot.config — 配置

```lua
-- 读取 plugin.toml [config] 中的值, 不存在时返回默认值
local val = neobot.config.get("key", "默认值")
```

## neobot.register — 注册器

### 注册命令

```lua
neobot.register.command("命令名", "权限等级", function(args)
    -- args 是字符串数组, args[1] 是第一个参数
    return "回复内容"  -- 返回 nil 则不回复
end, { aliases = { "别名1", "别名2" } })
```

**参数说明：**
- `命令名` — 字符串（不含前缀）
- `权限等级` — `"user"` / `"admin"` / `"superuser"`
- `function(args)` — 处理函数，`args` 是 Lua table（数组）
- `{aliases = {...}}` — 可选，别名列表

### 注册消息 Hook

```lua
neobot.register.on_message(function(text)
    -- text 是消息的纯文本内容
    if string.find(text, "关键词") then
        return "匹配到关键词时的回复"
    end
    -- 返回 nil 则不回复
end)
```

每条消息都会触发所有插件的消息 Hook。

### 注册通知 Hook

```lua
neobot.register.on_notice("通知类型", function(notice_type)
    -- notice_type 是通知类型字符串
    return "回复内容"  -- 可选
end)
```

**支持的通知类型：**

| 类型 | 说明 |
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

## neobot.bot — Bot API

### call_api — 通用 API 调用

```lua
local result, err = neobot.bot.call_api("action", { key = "value" })
-- result.status / result.retcode / result.msg / result.data (JSON string)
```

### 消息发送

```lua
-- 发送私聊消息
local msg_id = neobot.bot.send_private_msg(user_id, message)
-- 发送群消息
local msg_id = neobot.bot.send_group_msg(group_id, message)
-- 点赞
neobot.bot.send_like(user_id, times)
-- 撤回消息
neobot.bot.delete_msg(message_id)
```

`message` 参数可以是字符串，也可以是消息段数组（见 `neobot.seg`）。

### 群组操作

```lua
-- 获取群列表
local groups = neobot.bot.get_group_list()  -- 返回 Lua table 数组

-- 获取群信息
local info = neobot.bot.get_group_info(group_id)

-- 获取群成员列表
local members = neobot.bot.get_group_member_list(group_id)

-- 获取群成员信息
local member = neobot.bot.get_group_member_info(group_id, user_id)

-- 踢出成员 (reject_add_request: 是否拒绝再次加群)
neobot.bot.group_kick(group_id, user_id, false)

-- 禁言成员 (duration: 秒, 默认 1800)
neobot.bot.group_ban(group_id, user_id, 600)

-- 设置群名片
neobot.bot.set_group_card(group_id, user_id, "新名片")

-- 全员禁言
neobot.bot.set_group_whole_ban(group_id, true)
```

### 账号/好友

```lua
-- 获取登录信息
local info = neobot.bot.get_login_info()

-- 获取自身 QQ
local self_id = neobot.bot.self_id()

-- 获取陌生人信息
local info = neobot.bot.get_stranger_info(user_id)

-- 获取好友列表
local friends = neobot.bot.get_friend_list()
```

### 媒体

```lua
local result = neobot.bot.can_send_image()
local result = neobot.bot.can_send_record()
local info = neobot.bot.get_image("file_path")
```

## neobot.seg — 消息段构造器

构造 OneBot 消息段，用于发送组合消息（文本+图片+@等）。

```lua
local seg = neobot.seg

-- 文本
seg.text("你好")

-- 图片 (支持本地路径/URL/base64)
seg.image("https://example.com/img.png")

-- @某人
seg.at(123456)

-- QQ 表情
seg.face(123)

-- 引用回复
seg.reply(message_id)

-- 语音
seg.record("file.mp3")

-- 视频
seg.video("file.mp4")

-- JSON 消息
seg.json('{"app":"com.tencent.structmsg",...}')

-- 合并转发节点
seg.node(user_id, "昵称", message_content)

-- 组合发送示例
neobot.bot.send_group_msg(group_id, {
    seg.reply(event_msg_id),
    seg.text("回复内容"),
    seg.image("https://example.com/pic.png")
})
```

## neobot.event — 事件上下文

只读访问当前事件信息。

```lua
local user_id = neobot.event.user_id()       -- 发送者 QQ
local group_id = neobot.event.group_id()      -- 群号 (私聊为 0)
local message_type = neobot.event.message_type() -- "private" 或 "group"
local raw_message = neobot.event.raw_message() -- 原始消息文本
local message_id = neobot.event.message_id()   -- 消息 ID
local self_id = neobot.event.self_id()         -- 自己的 QQ
local segments = neobot.event.segments()       -- 当前消息的消息段数组
```

> **注意**：事件上下文仅在事件处理期间有效（即 handler 函数执行期间）。

## neobot.util — 工具函数

```lua
-- 时间戳 (Unix 秒)
local ts = neobot.util.now()

-- 格式化时间 (类似 C strftime)
local s = neobot.util.date("%Y-%m-%d %H:%M:%S")

-- 休眠 (毫秒)
neobot.util.sleep(1000)

-- HTTP 请求
local body = neobot.util.http_get("https://api.example.com")
local body = neobot.util.http_post("https://api.example.com", '{"key":"val"}', "application/json")

-- Base64
local encoded = neobot.util.base64_encode("hello")
local decoded = neobot.util.base64_decode(encoded)

-- 哈希
local hash = neobot.util.md5("data")
local hash = neobot.util.sha256("data")

-- JSON
local json_str = neobot.util.json_encode({ key = "value" })
local tbl = neobot.util.json_decode('{"key":"value"}')
```

## neobot.perm — 权限检查

```lua
-- 检查用户是否满足权限
local ok = neobot.perm.check(user_id, group_id, role, "admin")

-- 检查是否为超级用户
local ok = neobot.perm.is_super(user_id)
```

## neobot.redis — Redis (可选)

仅在配置了 Redis 连接后可用。检查 `neobot.redis.available()` 判断。

```lua
if not neobot.redis.available() then return end

neobot.redis.set("key", "value", 3600)  -- 设置 (含 TTL 秒)
local val = neobot.redis.get("key")      -- 获取
neobot.redis.del("key1", "key2")        -- 删除
local exists = neobot.redis.exists("key") -- 是否存在
local count = neobot.redis.incr("counter") -- 自增

-- Hash
neobot.redis.hset("hash_key", { field1 = "val1", field2 = "val2" })
local val = neobot.redis.hget("hash_key", "field1")
local all = neobot.redis.hgetall("hash_key")

-- List
neobot.redis.lpush("list_key", "a", "b")
neobot.redis.rpush("list_key", "c")
local val = neobot.redis.lpop("list_key")
local items = neobot.redis.lrange("list_key", 0, -1)
local len = neobot.redis.llen("list_key")
```

## neobot.mysql — MySQL (可选)

仅在配置了 MySQL 连接后可用。

```lua
if not neobot.mysql.available() then return end

-- 查询多条
local rows = neobot.mysql.query("SELECT * FROM users WHERE age > ?", 18)
-- 返回 table 数组, 每行为 { column = value }

-- 查询单条
local row = neobot.mysql.query_one("SELECT * FROM users WHERE id = ?", 1)

-- 执行修改
local affected = neobot.mysql.exec("INSERT INTO users (name) VALUES (?)", "张三")
```

## neobot.render — 图片渲染 (可选)

仅在配置了浏览器渲染后可用。

```lua
if not neobot.render.available() then return end

-- HTML 渲染
local base64_img = neobot.render.html("<h1>Hello</h1>", 800)  -- 宽度

-- URL 渲染 (网页截图)
local base64_img = neobot.render.url("https://example.com", 1280)

-- 模板渲染 (Go template 语法)
local base64_img = neobot.render.template("<h1>{{.title}}</h1>", { title = "Hello" }, 800)

-- 返回 base64:// 格式, 可直接作为图片发送
neobot.bot.send_group_msg(group_id, neobot.seg.image(base64_img))
```

## Lua 插件完整示例

```
plugins_lua/myplugin/
├── plugin.toml
└── plugin.lua
```

`plugin.toml`：

```toml
name = "myplugin"
version = "1.0.0"
author = "作者"
description = "我的插件"
usage = "/mycmd <参数>"
permission = "user"
runtime = "lua"

[config]
greeting = "你好"
```

`plugin.lua`：

```lua
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

### Lua 插件依赖

在 `plugin.toml` 中声明依赖的 Lua 库：

```toml
[dependencies]
lua = ["string-utils"]   # 校验 lib/string-utils.lua 存在
local = ["./lib"]        # 设置 package.path
```

然后在代码中加载：

```lua
package.path = neobot.package_paths .. ";./?.lua;" .. (package.path or "")
local str = require("string-utils")
```

---

# Python SDK 参考

Python 插件通过 `neobot_sdk` 包访问 SDK。每个 Python 插件在独立的子进程中运行，通过 JSON-RPC over stdio 与 Go 核心通信。

## 入口文件

```python
from neobot_sdk import command, on_message, on_notice


class Plugin:
    """插件类 - 框架自动实例化"""

    @command(name="pycmd", aliases=["pc"])
    async def my_command(self, sdk, params):
        args = params.get("args", [])
        return f"收到: {' '.join(args)}"

    @on_message
    async def on_text(self, sdk, params):
        text = params.get("text", "")
        if "python" in text.lower():
            return "[关键词检测]"
        return None

    @on_notice
    async def on_notify(self, sdk, params):
        notice_type = params.get("noticeType", "")
        sdk.log.info(f"通知: {notice_type}")
```

## Handler 签名

### 命令 Handler

```python
@command(name="命令名", permission="user", aliases=["别名1"])
async def handler(self, sdk, params):
    args = params.get("args", [])   # 参数列表
    # sdk.bot / sdk.event / sdk.seg 可用
    return "回复文本"  # 或 None 不回复
```

### 消息 Hook

```python
@on_message
async def handler(self, sdk, params):
    text = params.get("text", "")   # 纯文本消息
    return "回复"  # 或 None
```

### 通知 Hook

```python
@on_notice
async def handler(self, sdk, params):
    notice_type = params.get("noticeType", "")
    # 处理通知
```

## sdk.bot — Bot API

```python
# 通用 API 调用
result = sdk.bot.call_api("action", {"key": "value"})

# 消息
sdk.bot.send_private_msg(user_id, message)
sdk.bot.send_group_msg(group_id, message)
sdk.bot.send_msg(user_id=123, message="hello")     # 自动判断私聊/群聊
sdk.bot.delete_msg(message_id)
sdk.bot.get_msg(message_id)
sdk.bot.send_like(user_id, times=1)

# 群组
sdk.bot.get_group_list()
sdk.bot.get_group_info(group_id)
sdk.bot.get_group_member_list(group_id)
sdk.bot.get_group_member_info(group_id, user_id)
sdk.bot.group_kick(group_id, user_id, reject_add_request=False)
sdk.bot.group_ban(group_id, user_id, duration=1800)
sdk.bot.set_group_card(group_id, user_id, card="")
sdk.bot.set_group_name(group_id, group_name)
sdk.bot.set_group_whole_ban(group_id, enable=True)

# 好友
sdk.bot.get_stranger_info(user_id)
sdk.bot.get_friend_list()

# 账号
sdk.bot.get_login_info()

# 媒体
sdk.bot.can_send_image()
sdk.bot.can_send_record()
sdk.bot.get_image(file)
```

## sdk.seg — 消息段构造器

```python
seg = sdk.seg

seg.text("文本内容")
seg.image("https://example.com/img.png")  # URL / 本地路径 / base64
seg.at(user_id)
seg.face(face_id)
seg.reply(message_id)
seg.record("file.mp3")
seg.video("file.mp4")
seg.json(json_data)
seg.node(user_id, "昵称", content)  # 合并转发节点

# 组合发送示例
await sdk.bot.send_group_msg(group_id, [
    sdk.seg.reply(msg_id),
    sdk.seg.text("回复"),
    sdk.seg.image("pic.png")
])
```

## sdk.event — 事件上下文

```python
sdk.event.user_id       # int: 发送者 QQ
sdk.event.group_id      # int: 群号 (私聊为 0)
sdk.event.message_type  # str: "private" / "group"
sdk.event.raw_message   # str: 原始消息
sdk.event.message_id    # int: 消息 ID
sdk.event.self_id       # int: 自己的 QQ
sdk.event.segments      # list: 消息段数组
```

## sdk.log — 日志

```python
sdk.log.debug("调试")
sdk.log.info("信息")
sdk.log.warning("警告")
sdk.log.error("错误")
```

日志会输出到 NeoBot 日志系统，并带有相应级别。

## Python 插件依赖管理

在 `plugin.toml` 中声明 pip 依赖：

```toml
runtime = "python"

[dependencies]
python = ["requests>=2.31", "pillow"]
```

启动时 NeoBot 会自动：
1. 创建 venv 虚拟环境（如果 `use_venv = true`）
2. 安装声明的 pip 包（有缓存，依赖未变则跳过）
3. 在 venv 环境中运行插件

插件中可以自由 import 已安装的包：

```python
import requests
from PIL import Image

class Plugin:
    @command(name="fetch")
    async def fetch(self, sdk, params):
        r = requests.get(params["args"][0])
        return r.text[:200]
```

---

## 热重载

修改 `plugins_lua/<name>/plugin.lua` 或 `plugins_lua/<name>/plugin.toml` 后，文件监视器会在 **500ms 防抖**后自动重新加载对应插件。

Python 插件同样支持热重载：修改文件后会自动重启对应子进程。

无需手动操作，无需重启 NeoBot。

## 构建与运行

### 开发

```bash
go build -o neobot.exe ./cmd/neobot
.\neobot.exe
```

### 生产部署

```bash
# 编译
go build -ldflags="-s -w" -o neobot.exe ./cmd/neobot

# 确保配置文件存在
copy config.toml.example config.toml

# 创建日志目录
mkdir logs

# 启动
.\neobot.exe
```

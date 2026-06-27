# 插件基础

## plugin.toml 格式

每个插件是一个独立的目录，包含一个 `plugin.toml` 元信息文件和一个入口文件（`plugin.lua` 或 `plugin.py`）。目录名以 `_` 或 `.` 开头的会被跳过。

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

## 事件处理流程

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
  e. 子进程发送 ready 确认就绪
  f. Go 端收到 ready 后注册命令和 Hook

运行时:
  - 事件到达 → Router.Dispatch() → 匹配命令/Hook → 调用 handler

热重载:
  - fsnotify 监听文件变更 (Write/Create/Remove)
  - 500ms 防抖
  - Manager.Reload(name) → Unregister → Runtime.Unload → Runtime.Load

关闭:
  a. Manager.Close() → 各 Runtime.Close()
  b. Lua: L.Close() 关闭 VM
  c. Python: 发送 shutdown → 等待退出 → KILL
```

## 权限系统

NeoBot 使用三级权限模型：

| 等级 | 说明 | 判定方式 |
|---|---|---|
| `user` | 普通用户 | 所有用户 |
| `admin` | 管理员 | 群主/群管理 或 在 `admin_groups` 中 |
| `superuser` | 超级用户 | 在 `superusers` 列表中 |

权限检查逻辑：

```
if required == user     → 直接通过
if user in superusers   → 直接通过
if required == superuser → 拒绝 (非超级用户)
if role is owner/admin  → 通过
if group in admin_groups → 通过
否则 → 拒绝
```

### 使用

- **plugin.toml**: `permission = "admin"` — 插件所有命令的默认权限
- **命令注册**: `neobot.register.command("name", "superuser", handler)` — 命令级别覆盖
- **运行时检查**: `neobot.perm.check(user_id, group_id, role, "admin")`

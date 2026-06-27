# 架构概览

NeoBot 采用**三层插件架构**，从底层到上层依次为：Go 原生核心框架、Python 子进程运行时、Lua 嵌入式脚本运行时。

```
┌──────────────────────────────────────────────────────────────────┐
│                      NeoBot Go (主进程)                            │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │               Layer 1 — Go Native Core (核心框架)            │  │
│  │                                                            │  │
│  │  ┌──────────┐  ┌──────────┐  ┌───────────────┐            │  │
│  │  │ WS Client│  │  Bot     │  │  Permission   │            │  │
│  │  │ (NapCat) │  │ (API聚合)│  │  (三级权限)    │            │  │
│  │  └────┬─────┘  └────┬─────┘  └───────┬───────┘            │  │
│  │       │             │               │                      │  │
│  │  ┌────┴─────────────┴───────────────┴────────────────┐     │  │
│  │  │              Router (事件分发)                      │     │  │
│  │  │  Message → MessageHooks → CommandLookup → Handler │     │  │
│  │  │  Notice  → NoticeHooks                            │     │  │
│  │  └──────────────────────┬────────────────────────────┘     │  │
│  │                         │                                  │  │
│  │  ┌──────────────────────┴────────────────────────────┐     │  │
│  │  │  Plugin Manager  ─── Registry ─── File Watcher    │     │  │
│  │  │  (扫描目录 / 元数据解析 / 运行时调度 / 热重载)        │     │  │
│  │  └──────┬──────────────────────────────────┬────────┘     │  │
│  └─────────┼──────────────────────────────────┼──────────────┘  │
│            │                                  │                 │
│  ┌─────────┴────────┐              ┌──────────┴──────────┐      │
│  │ Layer 2           │              │ Layer 3              │      │
│  │ Python Runtime    │              │ Lua Runtime          │      │
│  │ (JSON-RPC/stdio)  │              │ (嵌入式 gopher-lua)   │      │
│  │                   │              │                      │      │
│  │ 子进程 × N        │              │ VM × N               │      │
│  │ venv 隔离         │              │ 零序列化开销          │      │
│  │ pip 依赖管理      │              │ require() 本地库      │      │
│  └───────────────────┘              └──────────────────────┘      │
│                                                                  │
│  可选服务: Redis │ MySQL │ Browser(HTML→图片渲染)                   │
└──────────────────────────────────────────────────────────────────┘
```

## Layer 1 — Go Native Core（核心框架层）

这是直接编译进 `neobot` 二进制的 Go 代码，是插件的**运行基石**，而非插件本身。它提供：

| 组件 | 文件 | 职责 |
|------|------|------|
| **Registry** | `internal/plugin/registry.go` | 统一的命令/事件注册表，线程安全 |
| **Manager** | `internal/plugin/manager.go` | 扫描插件目录，解析 `plugin.toml`，按 `runtime` 字段调度到对应运行时 |
| **Router** | `internal/plugin/router.go` | 事件入口：消息先走 MessageHook，再查命令表；Notice 走 NoticeHook |
| **Watcher** | `internal/plugin/watcher.go` | fsnotify 监听 `.lua` / `.toml` 变更，500ms 防抖自动热重载 |
| **Host** | `internal/plugin/runtime/runtime.go` | 依赖注入容器：Bot API、权限检查、Redis、MySQL、渲染、事件上下文 |
| **Deps** | `internal/plugin/deps/manager.go` | pip 依赖安装 + Lua 本地库校验 |

核心代码位于 `internal/plugin/`，通过 `Runtime` 接口连接 Layer 2 和 Layer 3。

## Layer 2 — Python Runtime（Python 子进程层）

- 每个插件**一个独立子进程**，崩溃不影响其他插件
- 通信协议：**JSON-RPC over stdio**（stdin/stdout 逐行 JSON）
- 使用 **venv 虚拟环境**隔离依赖，支持 `uv` 加速安装
- Go 侧入口：`internal/plugin/runtime/python.go` → 子进程管理：`internal/plugin/pythonproc/proc.go`
- Python 侧宿主：`shim/pyplugin_host.py`
- SDK 包：`shim/neobot_sdk/`（`Plugin` 基类 + 装饰器）

## Layer 3 — Lua Runtime（Lua 嵌入式脚本层）

- 每个插件**一个独立 `gopher-lua` VM**，在 Go 进程内执行
- SDK 以全局表 `neobot` 注入到 VM 中（`internal/plugin/runtime/lua_sdk.go`）
- 注册的命令/Hook 通过 **Go 闭包**桥接到 Registry，**零序列化开销**
- 支持 `require()` 加载本地 Lua 库（通过 `[dependencies].local`）

## 层级对比

| 层级 | 运行时 | 隔离方式 | 通信机制 | 适用场景 |
|------|--------|----------|----------|----------|
| **Layer 1** | Go 原生 | 无（框架本体） | 直接函数调用 | 核心功能扩展、高性能模块 |
| **Layer 2** | Python 子进程 | 独立进程 + venv | JSON-RPC/stdio | AI/ML、网络请求、复杂业务 |
| **Layer 3** | Lua 嵌入式 VM | 独立 VM 实例 | Go 闭包直接调用 | 轻量命令、快速原型、热更新 |

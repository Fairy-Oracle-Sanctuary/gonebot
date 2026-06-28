# NeoBot Go

基于 OneBot v11 协议的 QQ 机器人框架。Go 核心 + Python/Lua 双运行时。

> [!WARNING]  
> **实验阶段** — 本项目目前处于早期开发阶段，API 和架构可能发生较大变动，存在不稳定因素，请勿在生产环境部署。

## 三层插件架构

```
┌──────────────────────────────────────────────┐
│  Layer 1 — Go Native Core                    │
│  Registry │ Manager │ Router │ Host          │
├──────────────────────────────────────────────┤
│  Layer 2 — Python │ Layer 3 — Lua            │
│  子进程 + venv     │ 嵌入式 VM + 零序列化      │
└──────────────────────────────────────────────┘
```

- **Layer 1** — Go 核心框架，提供事件路由、权限校验、依赖注入、文件热重载
- **Layer 2** — Python 插件（独立子进程 + JSON-RPC/stdio + venv 隔离）
- **Layer 3** — Lua 插件（嵌入式 gopher-lua VM，零序列化开销）

详见 [架构文档](docs/architecture.md)。

## 特性

- **OneBot v11** 正向/反向 WebSocket
- **命令系统** + 别名 + 三级权限（user/admin/superuser）
- **热重载** — 修改插件文件后 500ms 内自动重载
- **依赖管理** — Python pip/venv 自动安装；Lua 本地库校验
- **服务集成** — Redis、MySQL、浏览器渲染（HTML→图片）

## 快速开始

```bash
go build -o neobot.exe ./cmd/neobot
copy config.toml.example config.toml
.\neobot.exe
```

详见 [快速开始](docs/quick-start.md)。

## 文档

| 文档 | 说明 |
|------|------|
| [快速开始](docs/quick-start.md) | 编译、配置、启动 |
| [架构概览](docs/architecture.md) | 三层插件架构详解 |
| [配置参考](docs/config.md) | `config.toml` 全部配置项 |
| [插件基础](docs/plugin-basics.md) | plugin.toml、生命周期、权限系统 |
| [Lua SDK](docs/lua-sdk.md) | Lua 插件完整 API 参考 |
| [Python SDK](docs/python-sdk.md) | Python 插件完整 API 参考 |
| [通信协议](docs/protocol.md) | JSON-RPC over stdio 协议规范 |
| [事件模型](docs/event-model.md) | OneBot v11 事件与消息段 |
| [最佳实践](docs/best-practices.md) | 命令设计、错误处理、性能、Python 注意事项 |

## 插件开发

**Lua** (`plugins_lua/<name>/plugin.lua`)：

```lua
neobot.register.command("hello", "user", function(args)
    return "Hello, " .. (args[1] or "World") .. "!"
end, { aliases = {"hi"} })

neobot.register.on_message(function(text)
    if string.find(text, "你好") then return "你也好！" end
end)
```

**Python** (`plugins_py/<name>/plugin.py`)：

```python
from neobot_sdk import Plugin, command, on_message

class MyPlugin(Plugin):
    async def on_init(self):
        """初始化钩子, 加载后自动调用"""

    @command(name="pyhello", aliases=["phi"])
    async def hello(self, sdk, params):
        args = params.get("args", [])
        return f"Hello, {args[0] or 'World'}!"

    @command(name="ping")
    async def ping(self, sdk, params):
        return "Pong!"

    @on_message
    async def on_text(self, sdk, params):
        if "你好" in params.get("text", ""):
            return "你也好！"
```

## 许可

[GNU Affero General Public License v3.0](LICENSE)

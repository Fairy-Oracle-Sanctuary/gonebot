# 配置参考

`config.toml` 完整配置项说明。

## [bot] — Bot 基本设置

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `self_id` | int | `0` | Bot QQ 号（0 = 自动获取） |
| `nickname` | string | `"NEO Bot"` | Bot 昵称 |
| `command_prefix` | []string | `["/", "!", "＃"]` | 命令前缀（会自动去除） |
| `ignore_self_message` | bool | `true` | 是否忽略自己发的消息 |
| `superusers` | []int64 | `[]` | 超级用户 QQ 号列表 |
| `admin_groups` | []int64 | `[]` | 管理员群号列表 |

## [napcat_ws] — 正向 WebSocket 连接

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `uri` | string | `"ws://127.0.0.1:30001"` | OneBot WS 地址 |
| `token` | string | `""` | Access Token |
| `reconnect_interval` | int | `5` | 重连间隔(秒) |
| `heartbeat_interval` | int | `30` | 心跳间隔(秒) |
| `api_request_timeout` | int | `30` | API 请求超时(秒) |

## [reverse_ws] — 反向 WebSocket 服务端

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 是否启用 |
| `host` | string | `"0.0.0.0"` | 监听地址 |
| `port` | int | `8080` | 监听端口 |
| `token` | string | `""` | Access Token |

## [plugins.lua] — Lua 插件

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `true` | 是否启用 |
| `dir` | string | `"plugins_lua"` | 插件目录 |
| `max_memory_mb` | int | `64` | 最大内存(MB) |
| `max_cpu_sec` | int | `5` | 最大 CPU 时间(秒) |

## [plugins.python] — Python 插件

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `enabled` | bool | `false` | 是否启用 |
| `dir` | string | `"plugins_py"` | 插件目录 |
| `python_bin` | string | `"python3"` | Python 可执行文件 |
| `shim_path` | string | `"shim/pyplugin_host.py"` | Shim 脚本路径 |

## [plugins.deps] — 依赖管理

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

## [redis] / [mysql] — 可选服务

见 `config.toml.example`。Redis/MySQL 仅在配置 `addr`/`dsn` 后才会连接，未配置时 SDK 中对应模块不可用。

## [browser] / [image] — 浏览器渲染

| 键 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `browser.enabled` | bool | `false` | 是否启用浏览器渲染 |
| `browser.pool_size` | int | `3` | 浏览器实例池大小 |
| `browser.headless` | bool | `true` | 无头模式 |
| `image.width` | int | `800` | 默认渲染宽度 |
| `image.quality` | int | `90` | 默认渲染质量 |
| `image.timeout_sec` | int | `30` | 渲染超时(秒) |

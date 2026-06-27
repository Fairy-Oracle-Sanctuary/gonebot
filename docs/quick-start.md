# 快速开始

## 前提条件

- **Go** 1.26+
- **Python** 3.10+（仅 Python 插件需要）
- 一个运行的 OneBot v11 实现（推荐 [NapCat](https://github.com/NapNeko/NapCatQQ)）

## 1. 编译

```bash
git clone <仓库地址> gonebot
cd gonebot
go build -o neobot.exe ./cmd/neobot
```

## 2. 配置

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

## 3. 启动 NapCat

确保 NapCat 已配置正向 WebSocket 服务并运行。

## 4. 启动 NeoBot

```bash
.\neobot.exe
```

首次运行会自动创建插件示例目录和默认配置文件。

## 构建与部署

### 开发环境

```bash
go build -o neobot.exe ./cmd/neobot
.\neobot.exe
```

### 生产部署

```bash
# 编译（去除调试信息、减小体积）
go build -ldflags="-s -w" -o neobot.exe ./cmd/neobot

# 确保配置文件存在
copy config.toml.example config.toml

# 创建日志目录
mkdir logs

# 启动
.\neobot.exe
```

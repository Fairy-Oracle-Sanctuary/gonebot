# Python SDK 参考

Python 插件通过 `neobot_sdk` 包访问 SDK。每个 Python 插件在独立子进程中运行，通过 JSON-RPC over stdio 与 Go 核心通信。

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
            return "[基于关键词的检测]"
        return None

    @on_notice
    async def on_notify(self, sdk, params):
        notice_type = params.get("noticeType", "")
        sdk.log.info(f"通知: {notice_type}")
```

## 装饰器

### @command

```python
@command(name: str | None = None, permission: str = "user", aliases: List[str] | None = None)
```

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `name` | str \| None | 函数名 | 命令名 |
| `permission` | str | `"user"` | 权限级别 |
| `aliases` | List[str] \| None | `None` | 别名列表 |

Handler 签名：

```python
async def handler(self, sdk, params: dict) -> str | None:
    args = params.get("args", [])   # List[str]
    return "回复文本"  # None 不回复
```

### @on_message

```python
@on_message
```

Handler 签名：

```python
async def handler(self, sdk, params: dict) -> str | None:
    text = params.get("text", "")   # str: 消息纯文本
    return "回复"  # None 不回复
```

### @on_notice

```python
@on_notice
```

Handler 签名：

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

    async def on_init(self) -> None:      # 初始化钩子 (可选)
        ...

    async def on_shutdown(self) -> None:   # 关闭钩子 (可选)
        ...

    def get_meta(self) -> dict:           # 获取元信息
        return {"name": ..., "version": ..., "description": ...}
```

## sdk 对象

在 handler 中通过 `sdk` 参数访问。

### sdk.bot — Bot API

通过 JSON-RPC `call_api` 调用 Go 端。

| 方法 | 签名 | 说明 |
|---|---|---|
| `call_api` | `(action, params?)` | 通用 OneBot API 调用 |
| `send_private_msg` | `(user_id, message)` | 发送私聊消息 |
| `send_group_msg` | `(group_id, message)` | 发送群消息 |
| `send_msg` | `(*, user_id=0, group_id=0, message)` | 自动判断发送方式 |
| `delete_msg` | `(message_id)` | 撤回消息 |
| `get_msg` | `(message_id)` | 获取消息详情 |
| `send_like` | `(user_id, times=1)` | 点赞 |
| `get_group_list` | `()` | 群列表 |
| `get_group_info` | `(group_id)` | 群信息 |
| `get_group_member_list` | `(group_id)` | 群成员列表 |
| `get_group_member_info` | `(group_id, user_id)` | 群成员信息 |
| `group_kick` | `(group_id, user_id, reject_add_request=False)` | 踢出成员 |
| `group_ban` | `(group_id, user_id, duration=1800)` | 禁言 |
| `set_group_card` | `(group_id, user_id, card="")` | 设置群名片 |
| `set_group_name` | `(group_id, group_name)` | 设置群名 |
| `set_group_whole_ban` | `(group_id, enable=True)` | 全员禁言 |
| `get_stranger_info` | `(user_id)` | 陌生人信息 |
| `get_friend_list` | `()` | 好友列表 |
| `get_login_info` | `()` | 登录信息 |
| `can_send_image` | `()` | 能否发送图片 |
| `can_send_record` | `()` | 能否发送语音 |
| `get_image` | `(file)` | 获取图片信息 |

### sdk.seg — 消息段构造器

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

```python
# 组合发送
sdk.bot.send_group_msg(group_id, [
    sdk.seg.reply(msg_id),
    sdk.seg.text("回复"),
    sdk.seg.at(user_id),
    sdk.seg.image("pic.png")
])
```

### sdk.event — 事件上下文

| 属性 | 类型 | 说明 |
|---|---|---|
| `sdk.event.user_id` | int | 发送者 QQ |
| `sdk.event.group_id` | int | 群号（私聊为 0） |
| `sdk.event.message_type` | str | `"private"` / `"group"` |
| `sdk.event.raw_message` | str | 原始消息文本 |
| `sdk.event.message_id` | int | 消息 ID |
| `sdk.event.self_id` | int | 自身 QQ |
| `sdk.event.segments` | List[dict] | 消息段数组 |

### sdk.log — 日志

```python
sdk.log.debug("调试")
sdk.log.info("信息")
sdk.log.warning("警告")
sdk.log.error("错误")
```

使用 Python 标准 `logging`，输出到 stderr，被 Go 端捕获并转发到主日志系统。

## 依赖管理

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

```python
import requests
from PIL import Image

class Plugin:
    @command(name="fetch")
    async def fetch(self, sdk, params):
        r = requests.get(params["args"][0])
        return r.text[:200]
```

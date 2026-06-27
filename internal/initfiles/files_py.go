package initfiles

// ---- Python 示例插件文件内容 ----

const pyEchoToml = `name = "pyecho"
version = "1.0.0"
author = "NeoBot Team"
description = "Python 示例插件: 复读机"
usage = "/pyecho <msg>"
permission = "user"
runtime = "python"
`

const pyEchoPy = `"""plugins_py/echo/plugin.py - Python 示例插件 (使用 neobot_sdk).

SDK 用法:
  sdk.bot.send_private_msg(...)   -- 调用 Bot API
  sdk.bot.get_login_info()        -- 通用 API 调用
  sdk.seg.text("hello")           -- 构造消息段
  sdk.event.user_id               -- 当前事件上下文
"""

from neobot_sdk import command, on_message, on_notice


class Plugin:
    """Python 示例插件."""

    @command(name="pyecho", aliases=["pye", "echo"])
    async def pyecho(self, sdk, params):
        """复读命令: /pyecho <内容>"""
        args = params.get("args", [])
        if not args:
            return "请输入要复读的内容, 例如 /pyecho 你好"

        msg = " ".join(args)
        # 可用 sdk.bot.send_msg() 主动发消息（非 handler 内调用不会死锁）
        return f"复读: {msg}"

    @on_message
    async def on_text(self, sdk, params):
        """消息 hook: 检测 python 关键词"""
        text = params.get("text", "")
        if "python" in text.lower():
            return "[Python SDK 检测到 python 关键词]"
        return None

    @on_notice
    async def on_notify(self, sdk, params):
        """通知 hook"""
        sdk.log.info(f"notice received: type={params.get('noticeType', '')}")
`

const pyWebfetchToml = `name = "pywebfetch"
version = "1.0.0"
author = "NeoBot Team"
description = "Python 插件示例: 使用 requests 拉取网页标题"
usage = "/pyfetch <url>"
permission = "user"
runtime = "python"

[dependencies]
python = ["requests>=2.31"]

[config]
timeout = 10
`

const pyWebfetchPy = `"""plugins_py/webfetch/plugin.py

演示依赖管理:
- plugin.toml [dependencies].python 声明 requests>=2.31
- 启动时 Manager 自动 pip install (有缓存)
- 插件内 import requests 直接可用
"""

from neobot_sdk import command

import requests  # ← 由依赖管理器自动安装


class Plugin:

    @command(name="pyfetch", aliases=["pyget"])
    async def fetch(self, sdk, params):
        args = params.get("args", [])
        if not args:
            return "用法: /pyfetch <url>"
        url = args[0]
        timeout = 10

        try:
            r = requests.get(url, timeout=timeout)
            import re
            m = re.search(r"<title[^>]*>(.*?)</title>", r.text, re.IGNORECASE | re.DOTALL)
            title = m.group(1).strip() if m else "(no title)"
            return f"标题: {title[:200]}"
        except Exception as e:
            return f"请求失败: {e}"
`

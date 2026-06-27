"""plugins_py/echo/plugin.py - Python 示例插件 (使用 neobot_sdk).

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

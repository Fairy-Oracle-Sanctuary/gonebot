# neobot_sdk - Python 插件 SDK (供插件作者使用)
#
# 使用方法:
#   from neobot_sdk import Plugin, command, on_message

from .plugin import Plugin, command, on_message, on_notice

__all__ = ["Plugin", "command", "on_message", "on_notice"]

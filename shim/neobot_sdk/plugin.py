"""neobot_sdk.plugin - Python 插件基类与装饰器."""

from __future__ import annotations
from typing import Any, Callable, List, Optional


def command(name: Optional[str] = None, permission: str = "user", aliases: Optional[List[str]] = None) -> Callable:
    """装饰器: 标记方法为命令处理函数.

    用法:
        class MyPlugin:
            @command(name="hello", permission="user", aliases=["hi"])
            async def hello(self, event, args):
                return "hi!"
    """
    def decorator(fn: Callable) -> Callable:
        fn._cmd_info = {
            "name": name or fn.__name__,
            "permission": permission,
            "aliases": aliases or [],
        }
        return fn
    return decorator


def on_message(fn: Callable) -> Callable:
    """装饰器: 标记方法为全局消息 hook."""
    fn._is_message_hook = True
    return fn


def on_notice(fn: Callable) -> Callable:
    """装饰器: 标记方法为通知 hook."""
    fn._is_notice_hook = True
    return fn


class Plugin:
    """插件基类. 继承并添加 @command / @on_message 方法."""

    name: str = ""
    version: str = "1.0.0"
    description: str = ""

    async def on_init(self) -> None:
        """可选: 初始化钩子."""

    async def on_shutdown(self) -> None:
        """可选: 关闭钩子."""

    def get_meta(self) -> dict:
        return {
            "name": self.name or self.__class__.__name__,
            "version": self.version,
            "description": self.description,
        }

package initfiles

// ---- Python shim 文件内容 ----

const shimPyPluginHost = `#!/usr/bin/env python3
"""pyplugin_host - Python 插件 shim (阶段 2: 单插件独立进程模式).

通信: JSON-RPC over stdio

Python 插件通过 SDK 调用 Go 端 API:
  await sdk.call_api("send_private_msg", {"user_id": 123, "message": "hello"})
  await sdk.send_private_msg(123, "hello")

Go -> Python: event dispatch (command / message / notice)
Python -> Go: call_api, log, ready, event_reply
"""
from __future__ import annotations

import argparse
import asyncio
import importlib.util
import inspect
import json
import logging
import os
import re
import sys
import threading
import time
import traceback
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional


logger = logging.getLogger("pyplugin_host")
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s [%(name)s] %(message)s",
    stream=sys.stderr,
)


# ---- 同步 stdout writer (避免 Windows asyncio pipe 问题) ----

class SyncWriter:
    def __init__(self, stream) -> None:
        self._stream = stream
        self._lock = threading.Lock()

    def write(self, data: bytes) -> None:
        with self._lock:
            try:
                self._stream.buffer.write(data)
                self._stream.buffer.flush()
            except Exception:
                pass


# ---- StdioRPC: JSON-RPC over stdio ----

class StdioRPC:
    """双向 JSON-RPC: send (fire-and-forget) + call (同步请求-响应)."""

    def __init__(self) -> None:
        self._reader: Optional[asyncio.StreamReader] = None
        self._writer: Optional[SyncWriter] = None
        self._running: bool = False
        self._pending_lock = threading.Lock()
        self._pending: Dict[str, threading.Event] = {}
        self._pending_results: Dict[str, Any] = {}
        self._id_counter: int = 0

    async def start(self) -> None:
        loop = asyncio.get_running_loop()
        self._reader = asyncio.StreamReader(loop=loop)
        self._writer = SyncWriter(sys.stdout)
        self._running = True

        # 用独立线程读 stdin, 通过 call_soon_threadsafe 喂给 asyncio.
        def _read_stdin() -> None:
            try:
                buf = sys.stdin.buffer
                while self._running:
                    line = buf.readline()
                    if not line:
                        break
                    try:
                        data = line
                    except Exception:
                        continue
                    loop.call_soon_threadsafe(self._reader.feed_data, data)
            except Exception:
                pass
            finally:
                loop.call_soon_threadsafe(self._reader.feed_eof)

        self._stdin_thread = threading.Thread(target=_read_stdin, daemon=True)
        self._stdin_thread.start()

    def send(self, method: str, params: Any = None, msg_id: Optional[str] = None) -> None:
        """Fire-and-forget 发送."""
        msg: Dict[str, Any] = {"method": method}
        if params is not None:
            msg["params"] = params
        if msg_id is not None:
            msg["id"] = msg_id
        line = json.dumps(msg, ensure_ascii=False) + "\n"
        if self._writer:
            self._writer.write(line.encode("utf-8"))

    def call(self, method: str, params: Any = None, timeout: float = 30.0) -> Any:
        """同步请求-响应. 可在任何线程调用."""
        with self._pending_lock:
            self._id_counter += 1
            msg_id = f"py_{self._id_counter}"
            event = threading.Event()
            self._pending[msg_id] = event

        self.send(method, params, msg_id)

        if not event.wait(timeout):
            raise TimeoutError(f"RPC call '{method}' timed out after {timeout}s")

        with self._pending_lock:
            result = self._pending_results.pop(msg_id, None)
            self._pending.pop(msg_id, None)

        if isinstance(result, dict) and "__error__" in result:
            raise RuntimeError(result["__error__"])
        return result

    def _resolve(self, msg_id: str, result: Any) -> None:
        with self._pending_lock:
            if msg_id in self._pending:
                self._pending_results[msg_id] = result
                self._pending[msg_id].set()

    def _handle_bot_reply(self, msg: Dict[str, Any]) -> None:
        """处理 Go 端返回的 bot_reply."""
        msg_id = msg.get("id")
        params = msg.get("params", {})
        if msg_id:
            self._resolve(msg_id, params)

    async def read_loop(self, handler: "MessageHandler") -> None:
        assert self._reader is not None
        while True:
            line = await self._reader.readline()
            if not line:
                logger.info("stdin closed, exiting")
                return
            try:
                msg = json.loads(line.decode("utf-8"))
            except json.JSONDecodeError as e:
                logger.warning(f"bad json: {e}")
                continue

            method = msg.get("method", "")
            if method == "bot_reply":
                self._handle_bot_reply(msg)
            else:
                await handler(msg)


# ---- Python SDK: 暴露给插件的 API ----

class MessageSegment:
    """消息段构造器 (与 OneBot 一致)."""

    @staticmethod
    def text(text: str) -> Dict[str, Any]:
        return {"type": "text", "data": {"text": str(text)}}

    @staticmethod
    def image(file: str) -> Dict[str, Any]:
        return {"type": "image", "data": {"file": str(file)}}

    @staticmethod
    def at(qq: int) -> Dict[str, Any]:
        return {"type": "at", "data": {"qq": str(qq)}}

    @staticmethod
    def face(id_: int) -> Dict[str, Any]:
        return {"type": "face", "data": {"id": str(id_)}}

    @staticmethod
    def reply(msg_id: int) -> Dict[str, Any]:
        return {"type": "reply", "data": {"id": str(msg_id)}}

    @staticmethod
    def record(file: str) -> Dict[str, Any]:
        return {"type": "record", "data": {"file": str(file)}}

    @staticmethod
    def video(file: str) -> Dict[str, Any]:
        return {"type": "video", "data": {"file": str(file)}}

    @staticmethod
    def json(data: str) -> Dict[str, Any]:
        return {"type": "json", "data": {"data": str(data)}}

    @staticmethod
    def node(user_id: int, nickname: str, content: Any) -> Dict[str, Any]:
        return {"type": "node", "data": {"user_id": str(user_id), "nickname": str(nickname), "content": content}}


class Bot:
    """Bot API - 通过 RPC 调用 Go 端 Bot."""

    def __init__(self, rpc: StdioRPC) -> None:
        self._rpc = rpc

    def call_api(self, action: str, params: Optional[Dict[str, Any]] = None) -> Any:
        """通用 API 调用."""
        return self._rpc.call("call_api", {"action": action, "params": params or {}})

    # -- 消息 --
    def send_private_msg(self, user_id: int, message: Any) -> Dict[str, Any]:
        return self.call_api("send_private_msg", {"user_id": user_id, "message": message})

    def send_group_msg(self, group_id: int, message: Any) -> Dict[str, Any]:
        return self.call_api("send_group_msg", {"group_id": group_id, "message": message})

    def send_msg(self, *, user_id: int = 0, group_id: int = 0, message: Any) -> Dict[str, Any]:
        if group_id:
            return self.send_group_msg(group_id, message)
        return self.send_private_msg(user_id, message)

    def delete_msg(self, message_id: int) -> Dict[str, Any]:
        return self.call_api("delete_msg", {"message_id": message_id})

    def get_msg(self, message_id: int) -> Dict[str, Any]:
        return self.call_api("get_msg", {"message_id": message_id})

    def send_like(self, user_id: int, times: int = 1) -> Dict[str, Any]:
        return self.call_api("send_like", {"user_id": user_id, "times": times})

    # -- 群组 --
    def get_group_list(self) -> List[Dict[str, Any]]:
        return self.call_api("get_group_list")

    def get_group_info(self, group_id: int) -> Dict[str, Any]:
        return self.call_api("get_group_info", {"group_id": group_id})

    def get_group_member_list(self, group_id: int) -> List[Dict[str, Any]]:
        return self.call_api("get_group_member_list", {"group_id": group_id})

    def get_group_member_info(self, group_id: int, user_id: int) -> Dict[str, Any]:
        return self.call_api("get_group_member_info", {"group_id": group_id, "user_id": user_id})

    def group_kick(self, group_id: int, user_id: int, reject_add_request: bool = False) -> Dict[str, Any]:
        return self.call_api("set_group_kick", {"group_id": group_id, "user_id": user_id, "reject_add_request": reject_add_request})

    def group_ban(self, group_id: int, user_id: int, duration: int = 1800) -> Dict[str, Any]:
        return self.call_api("set_group_ban", {"group_id": group_id, "user_id": user_id, "duration": duration})

    def set_group_card(self, group_id: int, user_id: int, card: str = "") -> Dict[str, Any]:
        return self.call_api("set_group_card", {"group_id": group_id, "user_id": user_id, "card": card})

    def set_group_whole_ban(self, group_id: int, enable: bool = True) -> Dict[str, Any]:
        return self.call_api("set_group_whole_ban", {"group_id": group_id, "enable": enable})

    def set_group_name(self, group_id: int, group_name: str) -> Dict[str, Any]:
        return self.call_api("set_group_name", {"group_id": group_id, "group_name": group_name})

    # -- 好友/陌生人 --
    def get_stranger_info(self, user_id: int) -> Dict[str, Any]:
        return self.call_api("get_stranger_info", {"user_id": user_id})

    def get_friend_list(self) -> List[Dict[str, Any]]:
        return self.call_api("get_friend_list")

    # -- 账号 --
    def get_login_info(self) -> Dict[str, Any]:
        return self.call_api("get_login_info")

    # -- 媒体 --
    def can_send_image(self) -> Dict[str, Any]:
        return self.call_api("can_send_image")

    def can_send_record(self) -> Dict[str, Any]:
        return self.call_api("can_send_record")

    def get_image(self, file: str) -> Dict[str, Any]:
        return self.call_api("get_image", {"file": file})


class Event:
    """当前事件上下文 (只读)."""

    def __init__(self) -> None:
        self.user_id: int = 0
        self.group_id: int = 0
        self.message_type: str = ""
        self.raw_message: str = ""
        self.message_id: int = 0
        self.self_id: int = 0
        self.segments: List[Dict[str, Any]] = []

    def _update(self, data: Dict[str, Any]) -> None:
        self.user_id = data.get("user_id", 0)
        self.group_id = data.get("group_id", 0)
        self.message_type = data.get("message_type", "")
        self.raw_message = data.get("raw_message", "")
        self.message_id = data.get("message_id", 0)
        self.self_id = data.get("self_id", 0)
        self.segments = data.get("segments", [])


class SDK:
    """注入到插件的 SDK. 每个 dispatch 重新绑定."""

    def __init__(self, rpc: StdioRPC) -> None:
        self.bot: Bot = Bot(rpc)
        self.seg: MessageSegment = MessageSegment()
        self.event: Event = Event()
        self._rpc = rpc
        self._log = logger  # 插件可用 sdk.log.info(...)

    def _update_event(self, data: Dict[str, Any]) -> None:
        self.event._update(data.get("event_ctx", {}))


# ---- 插件加载 ----

@dataclass
class CommandSpec:
    name: str
    permission: str = "user"
    aliases: List[str] = field(default_factory=list)


@dataclass
class PluginSpec:
    name: str
    version: str
    description: str
    author: str
    module: Any
    instance: Any
    commands: Dict[str, CommandSpec] = field(default_factory=dict)
    has_message_hook: bool = False
    has_notice_hook: bool = False


class PluginRegistry:
    def __init__(self) -> None:
        self.plugins: Dict[str, PluginSpec] = {}

    def load_one(self, plugin_dir: str) -> None:
        """加载单个插件."""
        root = Path(plugin_dir)
        name = root.name
        plugin_py = root / "plugin.py"
        plugin_toml = root / "plugin.toml"

        if not plugin_py.exists():
            raise FileNotFoundError(f"plugin.py not found: {plugin_py}")

        meta = self._read_metadata(plugin_toml)
        meta_name = meta.get("name", name)

        spec = importlib.util.spec_from_file_location(f"pyplugin_{meta_name}", plugin_py)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"failed to load spec: {name}")
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)

        instance = None
        for attr_name in ("Plugin", "plugin", "Main"):
            cls = getattr(module, attr_name, None)
            if cls is not None and isinstance(cls, type):
                try:
                    instance = cls()
                except Exception as e:
                    logger.error(f"failed to instantiate {attr_name}: {e}")
                break
        if instance is None:
            raise RuntimeError(f"{name}: no Plugin class")

        spec_obj = PluginSpec(
            name=meta_name,
            version=meta.get("version", "0.0.0"),
            description=meta.get("description", ""),
            author=meta.get("author", ""),
            module=module,
            instance=instance,
        )

        for attr_name in dir(instance):
            if attr_name.startswith("_"):
                continue
            attr = getattr(instance, attr_name, None)
            if not callable(attr):
                continue
            cmd_info = getattr(attr, "_cmd_info", None)
            if isinstance(cmd_info, dict):
                spec_obj.commands[attr_name] = CommandSpec(
                    name=cmd_info.get("name", attr_name),
                    permission=cmd_info.get("permission", "user"),
                    aliases=cmd_info.get("aliases", []),
                )
                continue
            if getattr(attr, "_is_message_hook", False):
                spec_obj.has_message_hook = True
            if getattr(attr, "_is_notice_hook", False):
                spec_obj.has_notice_hook = True

        self.plugins[meta_name] = spec_obj
        logger.info(f"loaded plugin: {meta_name} v{spec_obj.version} ({len(spec_obj.commands)} commands)")

    def _read_metadata(self, path: Path) -> Dict[str, str]:
        if not path.exists():
            return {}
        meta: Dict[str, str] = {}
        try:
            text = path.read_text(encoding="utf-8")
            for line in text.split("\n"):
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                m = re.match(r'^([\w_]+)\s*=\s*"([^"]*)"$', line)
                if m:
                    meta[m.group(1)] = m.group(2)
        except Exception as e:
            logger.warning(f"failed to read metadata {path}: {e}")
        return meta

    def ready_payload(self) -> Dict[str, Any]:
        if not self.plugins:
            return {"plugins": []}
        out = []
        for p in self.plugins.values():
            out.append({
                "name": p.name,
                "version": p.version,
                "description": p.description,
                "commands": [c.name for c in p.commands.values()],
                "permission": "user",
                "has_message_hook": p.has_message_hook,
                "has_notice_hook": p.has_notice_hook,
            })
        return {"plugins": out}


# ---- 装饰器 (供插件作者使用) ----

def command(name: Optional[str] = None, permission: str = "user", aliases: Optional[List[str]] = None) -> Callable:
    def decorator(fn: Callable) -> Callable:
        fn._cmd_info = {
            "name": name or fn.__name__,
            "permission": permission,
            "aliases": aliases or [],
        }
        return fn
    return decorator


def on_message(fn: Callable) -> Callable:
    fn._is_message_hook = True
    return fn


def on_notice(fn: Callable) -> Callable:
    fn._is_notice_hook = True
    return fn


# ---- Host 主循环 ----

class Host:
    def __init__(self, rpc: StdioRPC, registry: PluginRegistry) -> None:
        self.rpc = rpc
        self.registry = registry
        self.sdk = SDK(rpc)

    async def run(self) -> None:
        self.rpc.send("ready", self.registry.ready_payload())
        await self.rpc.read_loop(self.handle_msg)

    async def handle_msg(self, msg: Dict[str, Any]) -> None:
        method = msg.get("method", "")
        params = msg.get("params", {})
        msg_id = msg.get("id")

        try:
            if method == "event":
                ev = params.get("event", "?")
                logger.info(f"dispatching event: kind={ev} cmd={params.get('cmd','')} plugin={params.get('plugin','')} id={msg_id}")
                await self.dispatch_event(params, msg_id)
            elif method == "shutdown":
                logger.info("shutdown received")
                sys.exit(0)
            elif method == "ping":
                self.rpc.send("pong")
            else:
                logger.warning(f"unknown method: {method}")
        except Exception as e:
            logger.error(f"handle {method} failed: {e}")
            logger.info(traceback.format_exc())
            if msg_id:
                self.rpc.send("event_reply", {"error": str(e)}, msg_id)

    async def dispatch_event(self, params: Dict[str, Any], msg_id: Optional[str]) -> None:
        event_kind = params.get("event", "")
        logger.info(f"dispatch: kind={event_kind} cmd={params.get('cmd','')} plugin={params.get('plugin','')} id={msg_id}")
        
        reply_data: Any = {}
        try:
            plugin_name = params.get("plugin", "")
            spec = self.registry.plugins.get(plugin_name)
            if spec is None:
                logger.info(f"dispatch: plugin not found by name, trying fallback. registered={list(self.registry.plugins.keys())}")
                for n, s in self.registry.plugins.items():
                    if n == plugin_name or n == params.get("cmd"):
                        spec = s
                        plugin_name = n
                        logger.info(f"dispatch: fallback matched plugin='{n}'")
                        break
            if spec is None:
                logger.warning(f"plugin not found: '{plugin_name}', registered={list(self.registry.plugins.keys())}")
            else:
                # 更新 SDK 事件上下文
                self.sdk._update_event(params)

                if event_kind == "command":
                    cmd = params.get("cmd", "")
                    args = params.get("args", [])
                    logger.info(f"dispatch: command cmd={cmd} args={args} registered_commands={list(spec.commands.keys())}")
                    handler = None
                    for cname, cspec in spec.commands.items():
                        if cspec.name == cmd or cname == cmd:
                            handler = getattr(spec.instance, cname, None)
                            logger.info(f"dispatch: found handler cname={cname} handler={handler}")
                            break
                    if handler is not None:
                        logger.info(f"dispatch: calling handler with args={args}")
                        result = await self._call(handler, {"args": args, "sdk": self.sdk})
                        logger.info(f"dispatch: handler result={repr(result)}")
                        reply_data = {"reply": result}
                    else:
                        logger.warning(f"command not found: {cmd} in {plugin_name}, available={list(spec.commands.keys())}")

                elif event_kind == "message":
                    text = params.get("text", "")
                    for attr_name in dir(spec.instance):
                        if getattr(getattr(spec.instance, attr_name, None), "_is_message_hook", False):
                            handler = getattr(spec.instance, attr_name)
                            result = await self._call(handler, {"text": text, "sdk": self.sdk})
                            if result:
                                reply_data = {"reply": result}
                            break

                elif event_kind == "notice":
                    notice_type = params.get("noticeType", "")
                    for attr_name in dir(spec.instance):
                        if getattr(getattr(spec.instance, attr_name, None), "_is_notice_hook", False):
                            handler = getattr(spec.instance, attr_name)
                            await self._call(handler, {"noticeType": notice_type, "sdk": self.sdk})
                    reply_data = {}

        except Exception as e:
            logger.error(f"dispatch failed: {e}")
            logger.debug(traceback.format_exc())
            reply_data = {"error": str(e)}

        # 始终回复，避免 Go 端阻塞
        if msg_id:
            self.rpc.send("event_reply", reply_data, msg_id)
            logger.info(f"dispatch: sent event_reply id={msg_id} data={reply_data}")

    async def _call(self, handler: Callable, params: Dict[str, Any]) -> Any:
        """调用 handler, 兼容新旧两种签名.
           handler 始终是 bound method (self 已绑定), 直接调用即可.
           新: def handler(self, params)           -> handler(params)
           旧: def handler(self, sdk, params)      -> handler(sdk, params)
        """
        sig = inspect.signature(handler)
        nparams = len(sig.parameters)
        logger.info(f"_call: nparams={nparams} params_keys={list(params.keys())}")
        if nparams >= 2:
            result = handler(self.sdk, params)
        else:
            result = handler(params)
        logger.info(f"_call: result_type={type(result).__name__} is_coro={inspect.iscoroutine(result)}")
        if inspect.iscoroutine(result):
            return await result
        return result


# ---- 入口 ----

def main() -> int:
    parser = argparse.ArgumentParser(description="NeoBot Python plugin host")
    parser.add_argument("--plugin-dir", default="plugins_py", help="multi-plugin mode: scan dir")
    parser.add_argument("--plugin", help="single-plugin mode: plugin name (loads <dir>/<plugin>/plugin.py)")
    args = parser.parse_args()

    rpc = StdioRPC()
    registry = PluginRegistry()

    async def amain() -> None:
        await rpc.start()
        if args.plugin:
            plugin_dir = Path(args.plugin_dir) / args.plugin
            registry.load_one(str(plugin_dir))
        else:
            root = Path(args.plugin_dir)
            for entry in sorted(root.iterdir()):
                if not entry.is_dir() or entry.name.startswith("_"):
                    continue
                try:
                    registry.load_one(str(entry))
                except Exception as e:
                    logger.warning(f"skip {entry.name}: {e}")

        host = Host(rpc, registry)
        await host.run()

    try:
        asyncio.run(amain())
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())
`

const shimSDKInit = `# neobot_sdk - Python 插件 SDK (供插件作者使用)
#
# 使用方法:
#   from neobot_sdk import Plugin, command, on_message

from .plugin import Plugin, command, on_message, on_notice

__all__ = ["Plugin", "command", "on_message", "on_notice"]
`

const shimSDKPlugin = `"""neobot_sdk.plugin - Python 插件基类与装饰器."""

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
`

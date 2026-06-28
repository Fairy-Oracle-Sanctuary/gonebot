#!/usr/bin/env python3
"""pyplugin_host - Python 插件 shim (独立子进程模式).

通信: JSON-RPC over stdio

Python 插件通过 SDK 调用 Go 端 API:
  sdk.bot.send_private_msg(123, "hello")

Go -> Python: event dispatch (command / message / notice)
Python -> Go: call_api, log, ready, event_reply
"""
from __future__ import annotations

import asyncio
import importlib.util
import inspect
import json
import logging
import os
import sys
import threading
import traceback
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Dict, List, Optional

# 确保 shim/ 目录在 sys.path，以便导入 neobot_sdk
_shim_dir = Path(__file__).resolve().parent
if str(_shim_dir) not in sys.path:
    sys.path.insert(0, str(_shim_dir))

from neobot_sdk.plugin import Plugin, command, on_message, on_notice  # noqa: E402

logger = logging.getLogger("pyplugin_host")
logging.basicConfig(
    level=logging.DEBUG,
    format="%(asctime)s %(levelname)s [%(name)s] %(message)s",
    stream=sys.stderr,
)


# ---- 同步 stdout writer ----

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

        def _read_stdin() -> None:
            try:
                buf = sys.stdin.buffer
                while self._running:
                    line = buf.readline()
                    if not line:
                        break
                    loop.call_soon_threadsafe(self._reader.feed_data, line)
            except Exception:
                pass
            finally:
                loop.call_soon_threadsafe(self._reader.feed_eof)

        self._stdin_thread = threading.Thread(target=_read_stdin, daemon=True)
        self._stdin_thread.start()

    def send(self, method: str, params: Any = None, msg_id: Optional[str] = None) -> None:
        msg: Dict[str, Any] = {"method": method}
        if params is not None:
            msg["params"] = params
        if msg_id is not None:
            msg["id"] = msg_id
        line = json.dumps(msg, ensure_ascii=False) + "\n"
        if self._writer:
            self._writer.write(line.encode("utf-8"))

    def call(self, method: str, params: Any = None, timeout: float = 30.0) -> Any:
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


# ---- SDK 注入 ----

class MessageSegment:
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
    def __init__(self, rpc: StdioRPC) -> None:
        self._rpc = rpc

    def call_api(self, action: str, params: Optional[Dict[str, Any]] = None) -> Any:
        return self._rpc.call("call_api", {"action": action, "params": params or {}})

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

    def get_stranger_info(self, user_id: int) -> Dict[str, Any]:
        return self.call_api("get_stranger_info", {"user_id": user_id})

    def get_friend_list(self) -> List[Dict[str, Any]]:
        return self.call_api("get_friend_list")

    def get_login_info(self) -> Dict[str, Any]:
        return self.call_api("get_login_info")

    def can_send_image(self) -> Dict[str, Any]:
        return self.call_api("can_send_image")

    def can_send_record(self) -> Dict[str, Any]:
        return self.call_api("can_send_record")

    def get_image(self, file: str) -> Dict[str, Any]:
        return self.call_api("get_image", {"file": file})


class Event:
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
    def __init__(self, rpc: StdioRPC) -> None:
        self.bot: Bot = Bot(rpc)
        self.seg: MessageSegment = MessageSegment()
        self.event: Event = Event()
        self._rpc = rpc

    @property
    def log(self):
        return logger

    def _update_event(self, data: Dict[str, Any]) -> None:
        self.event._update(data.get("event_ctx", {}))


# ---- 插件加载 ----

@dataclass
class CommandSpec:
    name: str
    method_name: str
    permission: str = "user"
    aliases: List[str] = field(default_factory=list)


@dataclass
class PluginSpec:
    name: str
    module: Any
    instance: Any
    commands: Dict[str, CommandSpec] = field(default_factory=dict)
    has_message_hook: bool = False
    has_notice_hook: bool = False


class PluginRegistry:
    def __init__(self) -> None:
        self.plugins: Dict[str, PluginSpec] = {}

    def load_one(self, plugin_dir: str, meta: Optional[Dict[str, Any]] = None) -> PluginSpec:
        """加载单个插件。元信息由 Go 端通过 NEOBOT_META 环境变量注入。"""
        root = Path(plugin_dir)
        plugin_py = root / "plugin.py"

        if not plugin_py.exists():
            raise FileNotFoundError(f"plugin.py not found: {plugin_py}")

        if meta is None:
            meta = {}

        # 确保插件所在目录的父目录在 sys.path (用于 neobot_sdk import)
        parent = root.parent
        if str(parent) not in sys.path:
            sys.path.insert(0, str(parent))

        spec = importlib.util.spec_from_file_location(f"_plugin_{root.name}", plugin_py)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"failed to load spec: {root.name}")
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)

        # 发现 Plugin 实例: 优先 isinstance 检查, 兜底旧名字
        instance = self._discover_instance(module, root.name)

        # 调用初始化钩子
        init_fn = getattr(instance, "on_init", None)
        if callable(init_fn):
            try:
                result = init_fn()
                if inspect.iscoroutine(result):
                    import asyncio as _asyncio
                    try:
                        loop = _asyncio.get_running_loop()
                    except RuntimeError:
                        loop = None
                    if loop is not None:
                        _asyncio.ensure_future(result)
                    else:
                        logger.warning("on_init is async but no running loop")
            except Exception as e:
                logger.warning(f"on_init failed: {e}")

        # 扫描命令和 Hook
        spec_obj = PluginSpec(
            name=meta.get("name", root.name),
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
                    method_name=attr_name,
                    permission=cmd_info.get("permission", "user"),
                    aliases=cmd_info.get("aliases", []),
                )
                continue
            if getattr(attr, "_is_message_hook", False):
                spec_obj.has_message_hook = True
            if getattr(attr, "_is_notice_hook", False):
                spec_obj.has_notice_hook = True

        self.plugins[spec_obj.name] = spec_obj
        cmds = list(spec_obj.commands.keys())
        logger.info(f"loaded plugin: {spec_obj.name} (commands={cmds}, msg_hook={spec_obj.has_message_hook}, notice_hook={spec_obj.has_notice_hook})")
        return spec_obj

    def _discover_instance(self, module: Any, name: str) -> Any:
        """发现插件实例: 查找继承 Plugin 的类, 兜底兼容旧命名."""
        # 1. isinstance 检查 (优先)
        for attr_name in dir(module):
            cls = getattr(module, attr_name, None)
            if isinstance(cls, type) and issubclass(cls, Plugin) and cls is not Plugin:
                try:
                    return cls()
                except Exception as e:
                    logger.error(f"failed to instantiate {attr_name}: {e}")

        # 2. 兜底: 旧命名约定
        for attr_name in ("Plugin", "plugin", "Main"):
            cls = getattr(module, attr_name, None)
            if cls is not None and isinstance(cls, type):
                try:
                    return cls()
                except Exception as e:
                    logger.error(f"failed to instantiate {attr_name}: {e}")

        raise RuntimeError(f"{name}: no Plugin class found (inherit from neobot_sdk.Plugin or use class name 'Plugin')")

    def ready_payload(self) -> Dict[str, Any]:
        if not self.plugins:
            return {"plugins": []}
        out = []
        for p in self.plugins.values():
            out.append({
                "name": p.name,
                "commands": [
                    {"name": c.name, "permission": c.permission, "aliases": c.aliases}
                    for c in p.commands.values()
                ],
                "has_message_hook": p.has_message_hook,
                "has_notice_hook": p.has_notice_hook,
            })
        return {"plugins": out}


# ---- Host 主循环 ----

class Host:
    def __init__(self, rpc: StdioRPC, registry: PluginRegistry) -> None:
        self.rpc = rpc
        self.registry = registry
        self.sdk = SDK(rpc)

    async def run(self) -> None:
        self.rpc.send("ready", self.registry.ready_payload())
        logger.info("sent ready, entering read_loop")
        await self.rpc.read_loop(self.handle_msg)

    async def handle_msg(self, msg: Dict[str, Any]) -> None:
        method = msg.get("method", "")
        params = msg.get("params", {})
        msg_id = msg.get("id")

        try:
            if method == "event":
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
            logger.debug(traceback.format_exc())
            if msg_id:
                self.rpc.send("event_reply", {"error": str(e)}, msg_id)

    async def dispatch_event(self, params: Dict[str, Any], msg_id: Optional[str]) -> None:
        event_kind = params.get("event", "")
        plugin_name = params.get("plugin", "")
        spec = self.registry.plugins.get(plugin_name)

        # 兜底: 按注册的第一个插件匹配
        if spec is None and self.registry.plugins:
            spec = next(iter(self.registry.plugins.values()))
            plugin_name = spec.name

        if spec is None:
            logger.warning(f"plugin not found: '{plugin_name}'")
            if msg_id:
                self.rpc.send("event_reply", {}, msg_id)
            return

        self.sdk._update_event(params)
        reply_data: Any = {}

        try:
            if event_kind == "command":
                cmd = params.get("cmd", "")
                args = params.get("args", [])
                handler = None
                for cname, cspec in spec.commands.items():
                    if cspec.name == cmd or cname == cmd:
                        handler = getattr(spec.instance, cname, None)
                        break
                if handler is not None:
                    result = await self._call(handler, {"args": args})
                    reply_data = {"reply": result}
                else:
                    logger.warning(f"command not found: {cmd}, available={list(spec.commands.keys())}")

            elif event_kind == "message":
                text = params.get("text", "")
                for attr_name in dir(spec.instance):
                    if getattr(getattr(spec.instance, attr_name, None), "_is_message_hook", False):
                        handler = getattr(spec.instance, attr_name)
                        result = await self._call(handler, {"text": text})
                        if result:
                            reply_data = {"reply": result}
                        break

            elif event_kind == "notice":
                notice_type = params.get("noticeType", "")
                for attr_name in dir(spec.instance):
                    if getattr(getattr(spec.instance, attr_name, None), "_is_notice_hook", False):
                        handler = getattr(spec.instance, attr_name)
                        await self._call(handler, {"noticeType": notice_type})
                reply_data = {}

        except Exception as e:
            logger.error(f"dispatch failed: {e}")
            logger.debug(traceback.format_exc())
            reply_data = {"error": str(e)}

        if msg_id:
            self.rpc.send("event_reply", reply_data, msg_id)

    async def _call(self, handler: Callable, params: Dict[str, Any]) -> Any:
        """调用 handler，自动适配签名: handler(params) 或 handler(sdk, params)。"""
        # bound method: self 已绑定，只算其余参数
        sig = inspect.signature(handler)
        nparams = len(sig.parameters)

        if nparams == 1:
            result = handler(params)
        else:
            result = handler(self.sdk, params)

        if inspect.iscoroutine(result):
            return await result
        return result


# ---- 入口 ----

def main() -> int:
    """单插件模式。Go 端通过 NEOBOT_META 环境变量注入元信息。"""
    plugin_dir = os.environ.get("NEOBOT_PLUGIN_DIR", "plugins_py")
    plugin_name = os.environ.get("NEOBOT_PLUGIN_NAME", "")
    meta_raw = os.environ.get("NEOBOT_META", "")

    meta: Dict[str, Any] = {}
    if meta_raw:
        try:
            meta = json.loads(meta_raw)
        except json.JSONDecodeError:
            logger.warning("invalid NEOBOT_META JSON")

    if not plugin_name:
        logger.error("NEOBOT_PLUGIN_NAME not set")
        return 1

    rpc = StdioRPC()
    registry = PluginRegistry()

    async def amain() -> None:
        await rpc.start()

        try:
            plugin_path = str(Path(plugin_dir) / plugin_name)
            registry.load_one(plugin_path, meta)
        except Exception as e:
            logger.error(f"load failed: {e}")
            traceback.print_exc()
            rpc.send("ready", {"plugins": [], "error": str(e)})
            return

        host = Host(rpc, registry)
        await host.run()

    try:
        asyncio.run(amain())
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main())

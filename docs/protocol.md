# 通信协议

## Lua: 直接调用

Lua 插件通过 Go 闭包直接在 Go 内存空间中被调用，无序列化开销。

## Python: 帧协议 over stdio

Python 子进程与 Go 主进程通过 stdin/stdout 进行二进制帧通信。**帧格式: [4 字节大端 uint32 长度前缀] + [JSON payload]**。

相比旧的行分隔 JSON 协议，帧协议优势：
- **零扫描开销** — 读取固定 4 字节头即可确定载荷长度，无需逐字节扫描换行符
- **直接 I/O** — 使用 `io.ReadFull` 一次性读取完整载荷，消除 `bufio.Reader` 中间层
- **可靠分界** — 消息边界由长度明确界定，不受 JSON 内容中换行符干扰

Go 通过 `NEOBOT_META` / `NEOBOT_PLUGIN_DIR` / `NEOBOT_PLUGIN_NAME` 环境变量向 Python 注入元信息。

### 消息类型

```
Python → Go (就绪):
  {"method":"ready","params":{"plugins":[{"name":"myplugin","commands":[...],"has_message_hook":true,...}]}}

Go → Python (事件分发):
  {"method":"event","id":"req_xxx","params":{...}}

Python → Go (API 调用):
  {"method":"call_api","id":"py_xxx","params":{"action":"send_private_msg","params":{...}}}

Go → Python (API 返回值):
  {"method":"bot_reply","id":"py_xxx","params":{...}}

Python → Go (事件回复):
  {"method":"event_reply","id":"req_xxx","params":{...}}

Go → Python (心跳):
  {"method":"ping"}

Python → Go:
  {"method":"pong"}

Go → Python (关闭):
  {"method":"shutdown"}
```

### ready 载荷格式

Python 端启动完成后发送就绪消息，Go 端据此注册所有命令和 Hook。

```json
{
  "method": "ready",
  "params": {
    "plugins": [
      {
        "name": "myplugin",
        "commands": [
          {"name": "pycmd", "permission": "admin", "aliases": ["pc"]},
          {"name": "hello", "permission": "user", "aliases": []}
        ],
        "has_message_hook": true,
        "has_notice_hook": false
      }
    ]
  }
}
```

### 事件分发消息格式

**命令事件：**

```json
{
  "method": "event",
  "id": "req_abc123",
  "params": {
    "event": "command",
    "cmd": "echo",
    "args": ["参数1", "参数2"],
    "plugin": "myplugin",
    "event_ctx": {
      "user_id": 123456,
      "group_id": 789012,
      "message_type": "group",
      "raw_message": "/echo 参数1 参数2",
      "message_id": 5555,
      "self_id": 111111,
      "segments": [
        {"type": "text", "data": {"text": "/echo"}},
        {"type": "text", "data": {"text": " 参数1 参数2"}}
      ]
    }
  }
}
```

**消息事件：**

```json
{
  "method": "event",
  "params": {
    "event": "message",
    "text": "消息纯文本内容",
    "event_ctx": { "user_id": 123456, "group_id": 789012, "..." }
  }
}
```

**通知事件：**

```json
{
  "method": "event",
  "params": {
    "event": "notice",
    "noticeType": "group_increase",
    "event_ctx": { "user_id": 123456, "group_id": 789012, "..." }
  }
}
```

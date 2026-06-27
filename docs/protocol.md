# 通信协议

## Lua: 直接调用

Lua 插件通过 Go 闭包直接在 Go 内存空间中被调用，无序列化开销。

## Python: JSON-RPC over stdio

Python 子进程与 Go 主进程通过 stdin/stdout 进行 JSON-RPC 通信。消息格式为逐行 JSON。

### 消息类型

```
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

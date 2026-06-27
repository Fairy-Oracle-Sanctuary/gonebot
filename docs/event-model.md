# 事件模型与消息段

## 事件类型

OneBot v11 事件模型，四种 Post 类型：

```
PostMessage (message)   → MessageEvent
PostNotice  (notice)    → NoticeEvent
PostRequest (request)   → RequestEvent
PostMeta    (meta_event) → MetaEvent
```

## MessageEvent

```go
type MessageEvent struct {
    Event                               // time, self_id, post_type
    SubType     MessageSubType          // friend / normal / anonymous
    MessageID   int64
    UserID      int64
    MessageType MessageType             // private / group
    GroupID     int64
    Message     []MessageSegment
    RawMessage  string
    Font        int
    Sender      Sender
    Anonymous   *Anonymous
}
```

## NoticeEvent

```go
type NoticeEvent struct {
    Event
    NoticeType NoticeType   // group_upload / group_admin / group_decrease / group_increase / group_ban / friend_add / group_recall / friend_recall / notify
    GroupID    int64
    UserID     int64
    OperatorID int64
    SubType    string
    File       *File
    MessageID  int64
    TargetID   int64
    Duration   int
}
```

## Sender 结构

```go
type Sender struct {
    UserID   int64
    Nickname string
    Card     string   // 群名片
    Sex      string
    Age      int
    Area     string
    Role     string   // owner / admin / member
    Title    string
}
```

---

## 消息段规范

消息段是 OneBot v11 的标准消息格式，一个消息由多个消息段组成的数组表示。

### 消息段类型

| 类型 | type 字符串 | data 字段 | 说明 |
|---|---|---|---|
| 文本 | `text` | `{text: string}` | 纯文本 |
| 图片 | `image` | `{file: string}` | 图片文件/URL/base64 |
| @ | `at` | `{qq: string}` | @某人 |
| 表情 | `face` | `{id: string}` | QQ 表情ID |
| 引用 | `reply` | `{id: string}` | 引用回复 |
| 语音 | `record` | `{file: string}` | 语音文件 |
| 视频 | `video` | `{file: string}` | 视频文件 |
| JSON | `json` | `{data: string}` | JSON 卡片消息 |
| 合并转发 | `node` | `{user_id: string, nickname: string, content: any}` | 转发节点 |

### 使用示例

**Lua:**

```lua
neobot.bot.send_group_msg(group_id, {
    neobot.seg.reply(message_id),
    neobot.seg.text("回复内容"),
    neobot.seg.at(user_id),
    neobot.seg.image("https://example.com/pic.png")
})
```

**Python:**

```python
sdk.bot.send_group_msg(group_id, [
    sdk.seg.reply(message_id),
    sdk.seg.text("回复内容"),
    sdk.seg.at(user_id),
    sdk.seg.image("https://example.com/pic.png")
])
```

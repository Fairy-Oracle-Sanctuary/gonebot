// Package event 定义 OneBot v11 事件模型.
package event

import (
	"encoding/json"
	"fmt"
)

// PostType 上报类型.
type PostType string

const (
	PostMessage PostType = "message"
	PostNotice  PostType = "notice"
	PostRequest PostType = "request"
	PostMeta    PostType = "meta_event"
)

// Event 所有事件的公共字段.
type Event struct {
	Time     int64    `json:"time"`
	SelfID   int64    `json:"self_id"`
	PostType PostType `json:"post_type"`
}

// Sender 发送者信息.
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card,omitempty"`
	Sex      string `json:"sex,omitempty"`
	Age      int    `json:"age,omitempty"`
	Area     string `json:"area,omitempty"`
	Role     string `json:"role,omitempty"`
	Title    string `json:"title,omitempty"`
}

// Anonymous 匿名信息.
type Anonymous struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Flag string `json:"flag"`
}

// File 文件信息.
type File struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	BusID int64  `json:"busid,omitempty"`
	URL   string `json:"url,omitempty"`
}

// MessageSegmentType 消息段类型.
type MessageSegmentType string

const (
	SegText   MessageSegmentType = "text"
	SegFace   MessageSegmentType = "face"
	SegImage  MessageSegmentType = "image"
	SegAt     MessageSegmentType = "at"
	SegRecord MessageSegmentType = "record"
	SegVideo  MessageSegmentType = "video"
	SegReply  MessageSegmentType = "reply"
	SegJSON   MessageSegmentType = "json"
	SegNode   MessageSegmentType = "node"
)

// MessageSegment 消息段.
type MessageSegment struct {
	Type MessageSegmentType `json:"type"`
	Data map[string]any     `json:"data"`
}

// TextSegment 文本段.
func TextSegment(text string) MessageSegment {
	return MessageSegment{Type: SegText, Data: map[string]any{"text": text}}
}

// AtSegment @段.
func AtSegment(qq int64) MessageSegment {
	return MessageSegment{Type: SegAt, Data: map[string]any{"qq": qq}}
}

// ImageSegment URL 图片段.
func ImageSegment(url string) MessageSegment {
	return MessageSegment{Type: SegImage, Data: map[string]any{"file": url}}
}

// ImageBase64Segment base64 图片段.
func ImageBase64Segment(b64 string) MessageSegment {
	return MessageSegment{Type: SegImage, Data: map[string]any{"file": "base64://" + b64}}
}

// ReplySegment 引用回复段.
func ReplySegment(messageID int64) MessageSegment {
	return MessageSegment{Type: SegReply, Data: map[string]any{"id": messageID}}
}

// PlainText 提取消息中的纯文本.
func PlainText(segs []MessageSegment) string {
	var s string
	for _, seg := range segs {
		if seg.Type == SegText {
			if t, ok := seg.Data["text"].(string); ok {
				s += t
			}
		}
	}
	return s
}

// MessageType 消息类型.
type MessageType string

const (
	MsgPrivate MessageType = "private"
	MsgGroup   MessageType = "group"
)

// MessageSubType 消息子类型.
type MessageSubType string

const (
	MsgSubFriend    MessageSubType = "friend"
	MsgSubNormal    MessageSubType = "normal"
	MsgSubAnonymous MessageSubType = "anonymous"
)

// MessageEvent 消息事件.
type MessageEvent struct {
	Event
	SubType     MessageSubType   `json:"sub_type"`
	MessageID   int64            `json:"message_id"`
	UserID      int64            `json:"user_id"`
	MessageType MessageType      `json:"message_type"`
	GroupID     int64            `json:"group_id,omitempty"`
	Message     []MessageSegment `json:"message"`
	RawMessage  string           `json:"raw_message"`
	Font        int              `json:"font"`
	Sender      Sender           `json:"sender"`
	Anonymous   *Anonymous       `json:"anonymous,omitempty"`
}

// IsGroup 是否群消息.
func (e *MessageEvent) IsGroup() bool { return e.MessageType == MsgGroup }

// IsPrivate 是否私聊.
func (e *MessageEvent) IsPrivate() bool { return e.MessageType == MsgPrivate }

// NoticeType 通知类型.
type NoticeType string

const (
	NoticeGroupUpload   NoticeType = "group_upload"
	NoticeGroupAdmin    NoticeType = "group_admin"
	NoticeGroupDecrease NoticeType = "group_decrease"
	NoticeGroupIncrease NoticeType = "group_increase"
	NoticeGroupBan      NoticeType = "group_ban"
	NoticeFriendAdd     NoticeType = "friend_add"
	NoticeGroupRecall   NoticeType = "group_recall"
	NoticeFriendRecall  NoticeType = "friend_recall"
	NoticeNotify        NoticeType = "notify"
)

// NoticeEvent 通知事件.
type NoticeEvent struct {
	Event
	NoticeType NoticeType `json:"notice_type"`
	GroupID    int64      `json:"group_id,omitempty"`
	UserID     int64      `json:"user_id,omitempty"`
	OperatorID int64      `json:"operator_id,omitempty"`
	SubType    string     `json:"sub_type,omitempty"`
	File       *File      `json:"file,omitempty"`
	MessageID  int64      `json:"message_id,omitempty"`
	TargetID   int64      `json:"target_id,omitempty"`
	Duration   int        `json:"duration,omitempty"`
}

// RequestType 请求类型.
type RequestType string

const (
	ReqFriend RequestType = "friend"
	ReqGroup  RequestType = "group"
)

// RequestEvent 请求事件.
type RequestEvent struct {
	Event
	RequestType RequestType `json:"request_type"`
	UserID      int64       `json:"user_id"`
	GroupID     int64       `json:"group_id,omitempty"`
	SubType     string      `json:"sub_type,omitempty"`
	Comment     string      `json:"comment"`
	Flag        string      `json:"flag"`
}

// MetaEventType 元事件类型.
type MetaEventType string

const (
	MetaLifecycle MetaEventType = "lifecycle"
	MetaHeartbeat MetaEventType = "heartbeat"
)

// MetaEvent 元事件.
type MetaEvent struct {
	Event
	MetaEventType MetaEventType `json:"meta_event_type"`
	SubType       string        `json:"sub_type,omitempty"`
	Interval      int           `json:"interval,omitempty"`
}

// Any 事件的多态包装.
type Any struct {
	Type PostType

	Message *MessageEvent
	Notice  *NoticeEvent
	Request *RequestEvent
	Meta    *MetaEvent
}

// UnmarshalEvent 解析原始 JSON 为强类型事件.
func UnmarshalEvent(data []byte) (*Any, error) {
	var base Event
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("unmarshal base event: %w", err)
	}

	out := &Any{Type: base.PostType}
	switch base.PostType {
	case PostMessage:
		var e MessageEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal message event: %w", err)
		}
		out.Message = &e
	case "message_sent":
		// self-sent echo, same structure as message; router skips self messages
		var e MessageEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal message_sent event: %w", err)
		}
		out.Type = PostMessage
		out.Message = &e
	case PostNotice:
		var e NoticeEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal notice event: %w", err)
		}
		out.Notice = &e
	case PostRequest:
		var e RequestEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal request event: %w", err)
		}
		out.Request = &e
	case PostMeta:
		var e MetaEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal meta event: %w", err)
		}
		out.Meta = &e
	default:
		return nil, fmt.Errorf("unknown post_type: %q", base.PostType)
	}

	return out, nil
}

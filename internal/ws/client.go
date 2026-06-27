// Package ws 实现 OneBot v11 正向 WebSocket 客户端.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"neobot/core/internal/event"
	"neobot/core/internal/logger"
)

// APIRequest 发往 OneBot 的 API 请求.
type APIRequest struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
	Echo   string         `json:"echo,omitempty"`
}

// APIResponse OneBot 返回的响应.
type APIResponse struct {
	Status  string          `json:"status"`
	RetCode int             `json:"retcode"`
	Data    json.RawMessage `json:"data,omitempty"`
	Msg     string          `json:"msg,omitempty"`
	Echo    string          `json:"echo,omitempty"`
}

// IsOK 判断响应是否成功.
func (r *APIResponse) IsOK() bool { return r.Status == "ok" && r.RetCode == 0 }

// EventHandler 事件回调.
type EventHandler func(ctx context.Context, ev *event.Any)

// Client 正向 WS 客户端.
type Client struct {
	url    string
	token  string
	dialer *websocket.Dialer
	logger *slog.Logger

	conn   *websocket.Conn
	connMu sync.RWMutex

	pending sync.Map // echo -> chan APIResponse
	timeout time.Duration

	handler   EventHandler
	closeOnce sync.Once
	closed    chan struct{}
}

// Options 构造参数.
type Options struct {
	URL               string
	Token             string
	ReconnectInterval int
	APIRequestTimeout int
}

// NewClient 创建客户端.
func NewClient(opts Options) *Client {
	if opts.ReconnectInterval <= 0 {
		opts.ReconnectInterval = 5
	}
	if opts.APIRequestTimeout <= 0 {
		opts.APIRequestTimeout = 30
	}
	return &Client{
		url:     opts.URL,
		token:   opts.Token,
		dialer:  websocket.DefaultDialer,
		logger:  logger.Module("ws.client"),
		timeout: time.Duration(opts.APIRequestTimeout) * time.Second,
		closed:  make(chan struct{}),
	}
}

// OnEvent 设置事件回调.
func (c *Client) OnEvent(h EventHandler) { c.handler = h }

// Connect 启动连接循环 (阻塞直到 ctx 取消).
func (c *Client) Connect(ctx context.Context) error {
	backoff := 1.0
	maxBackoff := 60.0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.closed:
			return nil
		default:
		}

		if err := c.connectOnce(ctx); err != nil {
			c.logger.Warn("ws disconnected", "err", err.Error(), "retry_in_sec", backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-c.closed:
				return nil
			case <-time.After(time.Duration(backoff) * time.Second):
			}
			backoff = math.Min(backoff*2, maxBackoff)
			continue
		}
		backoff = 1.0
	}
}

func (c *Client) connectOnce(ctx context.Context) error {
	headers := http.Header{}
	if c.token != "" {
		headers.Set("Authorization", "Bearer "+c.token)
	}
	c.logger.Info("connecting to napcat", "url", c.url)

	conn, resp, err := c.dialer.DialContext(ctx, c.url, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial %s: %w (status=%d)", c.url, err, resp.StatusCode)
		}
		return fmt.Errorf("dial %s: %w", c.url, err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	c.logger.Info("ws connected")

	// ctx 取消时关闭连接
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-c.closed:
			_ = conn.Close()
		}
	}()

	return c.readLoop(conn)
}

func (c *Client) readLoop(conn *websocket.Conn) error {
	defer func() {
		c.connMu.Lock()
		c.conn = nil
		c.connMu.Unlock()
	}()

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var probe map[string]json.RawMessage
		if err := json.Unmarshal(data, &probe); err != nil {
			c.logger.Warn("decode probe failed", "err", err.Error())
			continue
		}

		// API 响应 (含 echo)
		if echoBytes, ok := probe["echo"]; ok {
			var echo string
			_ = json.Unmarshal(echoBytes, &echo)
			if echo != "" {
				if ch, ok := c.pending.LoadAndDelete(echo); ok {
					var resp APIResponse
					if err := json.Unmarshal(data, &resp); err == nil {
						ch.(chan APIResponse) <- resp
					}
				}
				continue
			}
		}

		// 事件上报
		if _, hasPost := probe["post_type"]; hasPost {
			ev, err := event.UnmarshalEvent(data)
			if err != nil {
				c.logger.Warn("decode event failed", "err", err.Error())
				continue
			}
			c.logEvent(ev)
			if c.handler != nil {
				go c.handler(context.Background(), ev)
			}
		}
	}
}

// CallAPI 同步调用 OneBot API.
func (c *Client) CallAPI(ctx context.Context, action string, params map[string]any) (*APIResponse, error) {
	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		c.logger.Warn("call_api: ws not connected", "action", action)
		return nil, errors.New("ws not connected")
	}

	echo := newEcho()
	req := APIRequest{Action: action, Params: params, Echo: echo}
	ch := make(chan APIResponse, 1)
	c.pending.Store(echo, ch)
	defer c.pending.Delete(echo)

	payload, err := json.Marshal(req)
	if err != nil {
		c.logger.Warn("call_api: marshal failed", "action", action, "err", err)
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	c.logger.Debug("call_api", "action", action, "echo", echo)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		c.logger.Warn("call_api: write failed", "action", action, "err", err)
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		c.logger.Warn("call_api: context done", "action", action)
		return nil, ctx.Err()
	case <-time.After(c.timeout):
		c.logger.Warn("call_api: timeout", "action", action)
		return nil, fmt.Errorf("api call timeout: action=%s", action)
	case resp := <-ch:
		c.logger.Debug("call_api: response", "action", action, "status", resp.Status, "retcode", resp.RetCode)
		return &resp, nil
	}
}

// Close 优雅关闭.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		c.connMu.Lock()
		defer c.connMu.Unlock()
		if c.conn != nil {
			err = c.conn.Close()
		}
	})
	return err
}

// logEvent 记录收到的事件.
func (c *Client) logEvent(ev *event.Any) {
	switch ev.Type {
	case event.PostMessage:
		m := ev.Message
		if m != nil {
			c.logger.Info("event received",
				"type", "message",
				"message_type", m.MessageType,
				"user_id", m.UserID,
				"group_id", m.GroupID,
				"raw", event.PlainText(m.Message),
			)
		}
	case event.PostNotice:
		n := ev.Notice
		if n != nil {
			c.logger.Info("event received",
				"type", "notice",
				"notice_type", n.NoticeType,
				"user_id", n.UserID,
				"group_id", n.GroupID,
			)
		}
	case event.PostRequest:
		r := ev.Request
		if r != nil {
			c.logger.Info("event received",
				"type", "request",
				"request_type", r.RequestType,
				"user_id", r.UserID,
				"group_id", r.GroupID,
				"comment", r.Comment,
			)
		}
	case event.PostMeta:
		m := ev.Meta
		if m != nil {
			c.logger.Info("event received",
				"type", "meta_event",
				"meta_type", m.MetaEventType,
			)
		}
	}
}

// newEcho 生成 echo ID.
func newEcho() string {
	var b [16]byte
	_, _ = randRead(b[:])
	return hexEncode(b[:])
}

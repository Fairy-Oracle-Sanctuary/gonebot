// Package reversews 反向 WebSocket 服务端.
//
// 与正向 WS (NapCat → NeoBot) 相反, ReverseWS 监听端口,
// 由 NapCat 或其他 OneBot 客户端主动连接 NeoBot, 然后 NeoBot
// 通过该连接推送事件和接收 API 调用.
//
// OneBot v11 反向 WS 协议:
//   - 客户端连接 ws://host:port/
//   - 服务端发送 {"action":"get_login_info","echo":...} 等 API
//   - 客户端发送 {"post_type":"message",...} 等事件
//   - 认证: HTTP Header "Authorization: Bearer <token>" (可选)
package reversews

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"neobot/core/internal/event"

	"github.com/gorilla/websocket"
)

// Config 反向 WS 配置.
type Config struct {
	Addr  string
	Token string
}

// EventHandler 事件处理回调. 由调用方 (main) 注入.
type EventHandler func(ctx context.Context, ev *event.Any)

// Server 反向 WS 服务端.
type Server struct {
	cfg     Config
	handler EventHandler
	logger  *slog.Logger

	upgrader websocket.Upgrader

	mu    sync.RWMutex
	conns map[*websocket.Conn]struct{}
	srv   *http.Server
}

// New 创建反向 WS 服务端.
func New(cfg Config, handler EventHandler) *Server {
	return &Server{
		cfg:     cfg,
		handler: handler,
		logger:  slog.Default().With("module", "reversews"),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		conns: make(map[*websocket.Conn]struct{}),
	}
}

// Start 启动 HTTP 服务并阻塞.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.wsHandler)
	mux.HandleFunc("/health", s.healthHandler)

	s.srv = &http.Server{
		Addr:         s.cfg.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	s.logger.Info("reversews listening", "addr", s.cfg.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		return err
	}
}

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	// Token 认证
	if s.cfg.Token != "" {
		token := r.Header.Get("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
		if token != s.cfg.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Warn("ws upgrade failed", "err", err.Error())
		return
	}

	s.mu.Lock()
	s.conns[conn] = struct{}{}
	s.mu.Unlock()

	remote := conn.RemoteAddr().String()
	s.logger.Info("ws client connected", "remote", remote)

	go s.readLoop(conn)
}

func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) readLoop(conn *websocket.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
		s.logger.Info("ws client disconnected")
	}()

	conn.SetReadLimit(256 * 1024) // 256KB max per message

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Warn("ws read error", "err", err.Error())
			}
			return
		}

		ev, err := event.UnmarshalEvent(raw)
		if err != nil {
			s.logger.Debug("parse event failed", "err", err.Error())
			continue
		}

		if s.handler != nil {
			s.handler(context.Background(), ev)
		}
	}
}

// SendJSON 向所有连接的客户端广播 JSON.
func (s *Server) SendJSON(v any) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastErr error
	for conn := range s.conns {
		if err := conn.WriteJSON(v); err != nil {
			lastErr = err
			s.logger.Warn("ws write error", "err", err.Error())
		}
	}
	return lastErr
}

// SendText 向所有连接的客户端广播文本.
func (s *Server) SendText(text string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastErr error
	for conn := range s.conns {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(text)); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Count 返回当前连接数.
func (s *Server) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.conns)
}

// SetHandler 设置事件处理器 (用于延迟注入).
func (s *Server) SetHandler(h EventHandler) {
	s.handler = h
}

func (s *Server) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 关闭所有连接
	s.mu.Lock()
	for conn := range s.conns {
		conn.Close()
		delete(s.conns, conn)
	}
	s.mu.Unlock()

	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

// Stop 停止服务.
func (s *Server) Stop() error {
	return s.shutdown()
}

var _ = fmt.Sprintf

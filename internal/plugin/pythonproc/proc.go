// Package pythonproc: 单插件 Python 子进程管理 (阶段 2).
//
// 每个插件一个独立的 Python 子进程, 配独立 venv.
//
//	neobot-go
//	  ├── pyplugin_host.py (env: NEOBOT_PLUGIN_NAME=echo)
//	  ├── pyplugin_host.py (env: NEOBOT_PLUGIN_NAME=webfetch)
//	  └── pyplugin_host.py (env: NEOBOT_PLUGIN_NAME=pyecho)
//
// 通信: 帧协议 over stdio — 4 字节大端长度前缀 + JSON payload.
package pythonproc

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// 帧协议: 4 字节大端长度 + JSON payload
const frameHeaderSize = 4

// Envelope JSON 消息信封.
type Envelope struct {
	Method string          `json:"method"`
	ID     string          `json:"id,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// LogParams 日志参数.
type LogParams struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// APIHandler 处理 Python 端的 API 调用 (bot / services).
type APIHandler interface {
	HandleAPI(ctx context.Context, action string, params map[string]any) (any, error)
}

// Proc 单个 Python 子进程.
type Proc struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	logger *slog.Logger

	pythonBin  string
	shimPath   string
	pluginDir  string
	pluginName string
	VenvPath   string
	metaJSON   string

	apiHandler APIHandler

	pendingAPIs sync.Map // req_id → chan json.RawMessage
	closed      chan struct{}

	readyCh      chan struct{}
	ReadyPayload json.RawMessage
}

// Config 进程配置.
type Config struct {
	PythonBin  string
	ShimPath   string
	PluginName string
	PluginDir  string
	VenvPath   string
	MetaJSON   string
	APIHandler APIHandler
}

// NewProc 创建子进程.
func NewProc(cfg Config) *Proc {
	return &Proc{
		pythonBin:  cfg.PythonBin,
		shimPath:   cfg.ShimPath,
		pluginDir:  cfg.PluginDir,
		pluginName: cfg.PluginName,
		VenvPath:   cfg.VenvPath,
		metaJSON:   cfg.MetaJSON,
		apiHandler: cfg.APIHandler,
		logger:     slog.Default().With("module", "pythonproc", "plugin", cfg.PluginName),
	}
}

// Start 启动子进程.
func (p *Proc) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return errors.New("proc already started")
	}

	python := p.pythonBin
	if p.VenvPath != "" {
		if runtime := os.Getenv("GOOS"); runtime == "windows" || runtime == "" {
			python = filepath.Join(p.VenvPath, "Scripts", "python.exe")
		} else {
			python = filepath.Join(p.VenvPath, "bin", "python")
		}
	}

	parentDir := filepath.Dir(p.pluginDir)
	dirName := filepath.Base(p.pluginDir)

	cmd := exec.CommandContext(ctx, python, p.shimPath)
	cmd.Dir = parentDir
	cmd.Env = append(os.Environ(),
		"PYTHONIOENCODING=utf-8",
		"PYTHONUNBUFFERED=1",
		"NEOBOT_PLUGIN_DIR="+parentDir,
		"NEOBOT_PLUGIN_NAME="+dirName,
	)
	if p.metaJSON != "" {
		cmd.Env = append(cmd.Env, "NEOBOT_META="+p.metaJSON)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	p.cmd = cmd
	p.stdin = stdin
	p.stdout = stdout
	p.closed = make(chan struct{})
	p.readyCh = make(chan struct{})

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start python: %w (is python installed?)", err)
	}

	go p.readLoop()

	p.logger.Info("proc started", "pid", cmd.Process.Pid, "python", python, "venv", p.VenvPath)
	return nil
}

// ---- 帧协议 I/O ----

// writeFrame 向 stdin 写入一帧: [4 字节长度][JSON payload].
func (p *Proc) writeFrame(data []byte) error {
	lenBuf := make([]byte, frameHeaderSize)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := p.stdin.Write(lenBuf); err != nil {
		return err
	}
	_, err := p.stdin.Write(data)
	return err
}

// readFrame 从 stdout 读取一帧, 返回 payload 字节.
func (p *Proc) readFrame() ([]byte, error) {
	lenBuf := make([]byte, frameHeaderSize)
	if _, err := io.ReadFull(p.stdout, lenBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf)
	if length > 64*1024*1024 { // 64MB 硬上限
		return nil, fmt.Errorf("frame too large: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(p.stdout, data); err != nil {
		return nil, err
	}
	return data, nil
}

// ---- 消息处理 ----

func (p *Proc) readLoop() {
	for {
		select {
		case <-p.closed:
			return
		default:
		}

		data, err := p.readFrame()
		if err != nil {
			if err != io.EOF {
				p.logger.Warn("read error", "err", err.Error())
			}
			p.logger.Info("subprocess exited")
			return
		}

		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			p.logger.Warn("decode msg failed", "err", err.Error(), "raw", string(data))
			continue
		}
		p.handle(&env)
	}
}

func (p *Proc) handle(env *Envelope) {
	switch env.Method {
	case "ready":
		p.logger.Info("plugin ready (ready received)")
		p.ReadyPayload = env.Params
		close(p.readyCh)

	case "log":
		var lp LogParams
		_ = json.Unmarshal(env.Params, &lp)
		lg := slog.Default().With("plugin", p.pluginName, "runtime", "python")
		switch lp.Level {
		case "debug":
			lg.Debug(lp.Msg)
		case "warn", "warning":
			lg.Warn(lp.Msg)
		case "error":
			lg.Error(lp.Msg)
		default:
			lg.Info(lp.Msg)
		}

	case "call_api":
		var req struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal(env.Params, &req)

		var resp Envelope
		resp.Method = "bot_reply"
		resp.ID = env.ID

		if p.apiHandler != nil {
			result, err := p.apiHandler.HandleAPI(context.Background(), req.Action, req.Params)
			if err != nil {
				resp.Params = mustJSON(map[string]any{"__error__": err.Error()})
				p.logger.Warn("call_api failed", "action", req.Action, "err", err)
			} else {
				resp.Params = mustJSON(map[string]any{"result": result})
				p.logger.Debug("call_api done", "action", req.Action)
			}
		} else {
			resp.Params = mustJSON(map[string]any{"__error__": "no API handler connected"})
			p.logger.Warn("call_api no handler", "action", req.Action)
		}
		_ = p.send(resp)

	case "event_reply":
		if ch, ok := p.pendingAPIs.LoadAndDelete(env.ID); ok {
			ch.(chan json.RawMessage) <- env.Params
		}

	case "pong":
		// heartbeat ack
	}
}

// Send 异步发送 (fire-and-forget).
func (p *Proc) Send(method string, params any) error {
	return p.send(Envelope{Method: method, Params: mustJSON(params)})
}

// Request 同步发送并等待响应.
func (p *Proc) Request(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	id := pyEcho()
	ch := make(chan json.RawMessage, 1)
	p.pendingAPIs.Store(id, ch)
	defer p.pendingAPIs.Delete(id)

	if err := p.send(Envelope{Method: method, ID: id, Params: mustJSON(params)}); err != nil {
		return nil, err
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	select {
	case <-time.After(timeout):
		return nil, errors.New("request timeout")
	case resp := <-ch:
		return resp, nil
	}
}

func (p *Proc) send(env Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stdin == nil {
		return errors.New("stdin not available")
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return p.writeFrame(data)
}

// Stop 优雅停止.
func (p *Proc) Stop() error {
	p.mu.Lock()
	if p.cmd == nil {
		p.mu.Unlock()
		return nil
	}
	_ = p.send(Envelope{Method: "shutdown"})
	cmd := p.cmd
	p.stdin = nil
	p.stdout = nil
	p.mu.Unlock()

	close(p.closed)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	p.mu.Lock()
	p.cmd = nil
	p.mu.Unlock()
	return nil
}

// PID 返回进程 ID.
func (p *Proc) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// WaitReady 等待 Python 端 ready 消息.
func (p *Proc) WaitReady(timeout time.Duration) (json.RawMessage, error) {
	select {
	case <-p.readyCh:
		return p.ReadyPayload, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for plugin ready")
	}
}

// ---- 事件转发 ----

// CallCommandEx 转发命令调用, 携带事件上下文.
func (p *Proc) CallCommandEx(cmd string, args []string, evtCtx map[string]any) any {
	resp, err := p.Request("event", map[string]any{
		"event":     "command",
		"cmd":       cmd,
		"args":      mustJSON(args),
		"event_ctx": evtCtx,
	}, 30*time.Second)
	if err != nil {
		p.logger.Warn("call cmd failed", "cmd", cmd, "err", err.Error())
		return nil
	}
	var data struct {
		Reply json.RawMessage `json:"reply"`
		Error string          `json:"error"`
	}
	_ = json.Unmarshal(resp, &data)
	if data.Error != "" {
		p.logger.Warn("python command error", "cmd", cmd, "error", data.Error)
	}
	if len(data.Reply) > 0 {
		var s string
		if err := json.Unmarshal(data.Reply, &s); err == nil {
			return s
		}
	}
	return nil
}

// CallMessageHookEx 转发消息 hook.
func (p *Proc) CallMessageHookEx(text string, evtCtx map[string]any) any {
	resp, err := p.Request("event", map[string]any{
		"event":     "message",
		"text":      text,
		"event_ctx": evtCtx,
	}, 30*time.Second)
	if err != nil {
		return nil
	}
	var data struct {
		Reply json.RawMessage `json:"reply"`
	}
	_ = json.Unmarshal(resp, &data)
	if len(data.Reply) > 0 {
		var s string
		if err := json.Unmarshal(data.Reply, &s); err == nil {
			return s
		}
	}
	return nil
}

// CallNoticeHookEx 转发通知 hook.
func (p *Proc) CallNoticeHookEx(noticeType string, evtCtx map[string]any) any {
	resp, err := p.Request("event", map[string]any{
		"event":      "notice",
		"noticeType": noticeType,
		"event_ctx":  evtCtx,
	}, 10*time.Second)
	if err != nil {
		return nil
	}
	var data struct {
		Reply json.RawMessage `json:"reply"`
	}
	_ = json.Unmarshal(resp, &data)
	if len(data.Reply) > 0 {
		var s string
		if err := json.Unmarshal(data.Reply, &s); err == nil {
			return s
		}
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func pyEcho() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Package pythonproc: 单插件 Python 子进程管理 (阶段 2).
//
// 每个插件一个独立的 Python 子进程, 配独立 venv.
//
//	neobot-go
//	  ├── pyplugin_host.py --plugin=echo    --venv=plugins_py/echo/venv
//	  ├── pyplugin_host.py --plugin=pwebfetch --venv=plugins_py/webfetch/venv
//	  └── pyplugin_host.py --plugin=pyecho   --venv=plugins_py/pyecho/venv
//
// 通信: JSON-RPC over stdio.
package pythonproc

import (
	"bufio"
	"context"
	"crypto/rand"
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

// Protocol 与原 PythonRuntime 共用, 放这里方便引用.
type Envelope struct {
	Method string          `json:"method"`
	ID     string          `json:"id,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Error  string          `json:"error,omitempty"`
}

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
	stdout *bufio.Reader
	logger *slog.Logger

	pythonBin  string
	shimPath   string
	pluginDir  string
	pluginName string
	VenvPath   string
	metaJSON   string // 插件元信息 JSON

	apiHandler APIHandler

	pendingAPIs sync.Map // req_id → chan json.RawMessage
	closed      chan struct{}

	// ready 等待
	readyCh      chan struct{}
	ReadyPayload json.RawMessage // Python ready 载荷 (命令列表等)
}

// Config 进程配置.
type Config struct {
	PythonBin  string
	ShimPath   string
	PluginName string
	PluginDir  string
	VenvPath   string     // 留空表示使用系统 Python
	MetaJSON   string     // 插件元信息 JSON (传给 Python 宿主)
	APIHandler APIHandler // Python 插件调用的 API 处理器
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

// Start 启动子进程并等待 init 确认.
func (p *Proc) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return errors.New("proc already started")
	}

	python := p.pythonBin
	if p.VenvPath != "" {
		// 使用 venv 内 python
		if runtime := os.Getenv("GOOS"); runtime == "windows" || runtime == "" {
			python = filepath.Join(p.VenvPath, "Scripts", "python.exe")
		} else {
			python = filepath.Join(p.VenvPath, "bin", "python")
		}
	}

	parentDir := filepath.Dir(p.pluginDir)
	dirName := filepath.Base(p.pluginDir)
	args := []string{p.shimPath}
	cmd := exec.CommandContext(ctx, python, args...)
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
	p.stdout = bufio.NewReader(stdout)
	p.closed = make(chan struct{})
	p.readyCh = make(chan struct{})

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start python: %w (is python installed?)", err)
	}

	go p.readLoop()

	p.logger.Info("proc started", "pid", cmd.Process.Pid, "python", python, "venv", p.VenvPath)
	return nil
}

func (p *Proc) readLoop() {
	for {
		select {
		case <-p.closed:
			return
		default:
		}

		line, err := p.stdout.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				p.logger.Warn("read error", "err", err.Error())
			}
			p.logger.Info("subprocess exited")
			return
		}
		var env Envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			p.logger.Warn("decode msg failed", "err", err.Error())
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
		// Python 插件调用 Go 端 Bot API
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
	data = append(data, '\n')
	_, err = p.stdin.Write(data)
	return err
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

// PID 返回进程 ID (用于诊断).
func (p *Proc) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// WaitReady 等待 Python 端发送 ready 消息, 返回载荷.
func (p *Proc) WaitReady(timeout time.Duration) (json.RawMessage, error) {
	select {
	case <-p.readyCh:
		return p.ReadyPayload, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for plugin ready")
	}
}

// 兼容旧接口 (PythonRuntime 沿用).

// CallCommand 转发命令调用 (兼容旧接口).
func (p *Proc) CallCommand(cmd string, args []string) any {
	return p.CallCommandEx(cmd, args, nil)
}

// CallCommandEx 转发命令调用, 携带事件上下文.
func (p *Proc) CallCommandEx(cmd string, args []string, evtCtx map[string]any) any {
	argsJSON := mustJSON(args)
	p.logger.Info("calling python command", "cmd", cmd, "args", args)
	resp, err := p.Request("event", map[string]any{
		"event":     "command",
		"cmd":       cmd,
		"args":      argsJSON,
		"event_ctx": evtCtx,
	}, 30*time.Second)
	if err != nil {
		p.logger.Warn("call cmd failed", "cmd", cmd, "err", err.Error())
		return nil
	}
	p.logger.Info("python command response", "cmd", cmd, "raw", string(resp))
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
			p.logger.Info("python command reply", "cmd", cmd, "reply", s)
			return s
		}
		p.logger.Info("python command reply (non-string)", "cmd", cmd, "reply", string(data.Reply))
	}
	p.logger.Info("python command no reply", "cmd", cmd)
	return nil
}

// CallMessageHook 转发消息 hook (兼容旧接口).
func (p *Proc) CallMessageHook(text string) any {
	return p.CallMessageHookEx(text, nil)
}

// CallMessageHookEx 转发消息 hook, 携带事件上下文.
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

// CallNoticeHook 转发通知 hook (兼容旧接口).
func (p *Proc) CallNoticeHook(noticeType string) {
	p.CallNoticeHookEx(noticeType, nil)
}

// CallNoticeHookEx 转发通知 hook, 携带事件上下文.
func (p *Proc) CallNoticeHookEx(noticeType string, evtCtx map[string]any) any {
	resp, err := p.Request("event", map[string]any{
		"event":      "notice",
		"noticeType": noticeType,
		"event_ctx":  evtCtx,
	}, 10*time.Second)
	if err != nil {
		return nil
	}
	// 通知 hook 可能返回 reply
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

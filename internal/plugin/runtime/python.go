// Package runtime: Python 子进程运行时 (阶段 2 - 进程隔离 + venv).
//
// 每个 Python 插件独立子进程 + 可选 venv.
// Python 插件通过 RPC call_api 调用 Go 端 Bot API.
// 事件上下文 (user_id, group_id 等) 通过 Go 的 Host.EventCtx 传入 Python.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"neobot/core/internal/event"
	"neobot/core/internal/permission"
	"neobot/core/internal/plugin/pythonproc"
)

// PythonRuntime 进程池: 每插件一个独立 pythonproc.Proc.
type PythonRuntime struct {
	logger *slog.Logger

	pythonBin      string
	shimPath       string
	sharedVenvPath string

	mu     sync.RWMutex
	procs  map[string]*pythonproc.Proc
	loaded map[string]*PluginInfo

	registry    RegistrySubset
	currentHost *Host // 当前宿主引用 (用于读取 EventCtx 和 Bot)
}

// PythonConfig 配置.
type PythonConfig struct {
	PythonBin      string
	ShimPath       string
	UseVenv        bool
	SharedVenvPath string
}

// NewPythonRuntime 创建 Python 运行时.
func NewPythonRuntime(cfg PythonConfig) *PythonRuntime {
	if cfg.PythonBin == "" {
		cfg.PythonBin = "python3"
	}
	if cfg.ShimPath == "" {
		cfg.ShimPath = "shim/pyplugin_host.py"
	}
	if !filepath.IsAbs(cfg.ShimPath) {
		if abs, err := filepath.Abs(cfg.ShimPath); err == nil {
			cfg.ShimPath = abs
		}
	}
	return &PythonRuntime{
		pythonBin:      cfg.PythonBin,
		shimPath:       cfg.ShimPath,
		sharedVenvPath: cfg.SharedVenvPath,
		procs:          make(map[string]*pythonproc.Proc),
		loaded:         make(map[string]*PluginInfo),
		logger:         slog.Default().With("module", "python.runtime"),
	}
}

// HandleAPI implements pythonproc.APIHandler.
func (r *PythonRuntime) HandleAPI(ctx context.Context, action string, params map[string]any) (any, error) {
	if r.currentHost == nil || r.currentHost.Bot == nil {
		return nil, errors.New("bot not connected")
	}
	return r.currentHost.Bot.CallAPI(ctx, action, params)
}

// Name 返回运行时名.
func (r *PythonRuntime) Name() string { return "python" }

// Load 加载单个 Python 插件.
func (r *PythonRuntime) Load(ctx context.Context, host *Host, dir, entry string, meta *Metadata) error {
	r.mu.Lock()
	if _, exists := r.procs[meta.Name]; exists {
		if old := r.procs[meta.Name]; old != nil {
			_ = old.Stop()
		}
		delete(r.procs, meta.Name)
		delete(r.loaded, meta.Name)
	}
	r.mu.Unlock()

	r.currentHost = host

	// 1. 依赖安装
	venvPath := ""
	if host != nil && host.Deps != nil && meta.Dependencies != nil && len(meta.Dependencies.Python) > 0 {
		if host.Deps.IsUseVenv() {
			if r.sharedVenvPath != "" {
				venvPath = r.sharedVenvPath
			} else {
				venvPath = filepath.Join(dir, "venv")
			}
			venvInfo, err := host.Deps.EnsureVenv(venvPath, meta.Name, meta.Dependencies.Python)
			if err != nil {
				return fmt.Errorf("ensure venv for %s: %w", meta.Name, err)
			}
			_ = venvInfo
		} else {
			if err := host.Deps.InstallPython(meta.Name, dir, meta.Dependencies.Python); err != nil {
				return fmt.Errorf("install python deps for %s: %w", meta.Name, err)
			}
		}
	}

	// 2. 创建子进程 (传入 APIHandler = 自己)
	proc := pythonproc.NewProc(pythonproc.Config{
		PythonBin:  r.pythonBin,
		ShimPath:   r.shimPath,
		PluginName: meta.Name,
		PluginDir:  dir,
		VenvPath:   venvPath,
		APIHandler: r,
	})

	if err := proc.Start(ctx); err != nil {
		return fmt.Errorf("start proc for %s: %w", meta.Name, err)
	}

	r.mu.Lock()
	r.procs[meta.Name] = proc
	r.loaded[meta.Name] = &PluginInfo{
		Name:        meta.Name,
		Version:     meta.Version,
		Description: meta.Description,
		Permission:  meta.Permission,
	}
	r.mu.Unlock()

	// 3. 注册命令和 hooks (使用闭包捕获 host.EventCtx)
	if host.Registry != nil {
		pluginName := meta.Name
		required := permission.ParseLevelSimple(meta.Permission)

		host.Registry.RegisterCommand(pluginName, pluginName, nil, required, func(args []string) any {
			proc := r.getProc(pluginName)
			if proc == nil {
				return "plugin not loaded"
			}
			// 构造事件上下文传给 Python
			evtCtx := r.buildEventCtx()
			return proc.CallCommandEx(pluginName, args, evtCtx)
		})

		// 消息 hook
		host.Registry.RegisterMessageHook(pluginName, func(text string) any {
			proc := r.getProc(pluginName)
			if proc == nil {
				return nil
			}
			evtCtx := r.buildEventCtx()
			return proc.CallMessageHookEx(text, evtCtx)
		})

		// 通知 hook (noticeType 从 EventCtx 的 RawMessage 取)
		host.Registry.RegisterNoticeHook(pluginName, "*", func(noticeType string) any {
			proc := r.getProc(pluginName)
			if proc == nil {
				return nil
			}
			evtCtx := r.buildEventCtx()
			return proc.CallNoticeHookEx(noticeType, evtCtx)
		})
	}

	r.logger.Info("plugin ready (isolated proc)",
		"plugin", meta.Name, "pid", proc.PID(), "venv", proc.VenvPath)
	return nil
}

// buildEventCtx 从 Host.EventCtx 构建传给 Python 的事件上下文.
func (r *PythonRuntime) buildEventCtx() map[string]any {
	ctx := map[string]any{}
	if r.currentHost == nil || r.currentHost.EventCtx == nil {
		return ctx
	}
	ec := r.currentHost.EventCtx
	ctx["user_id"] = ec.UserID
	ctx["group_id"] = ec.GroupID
	ctx["message_type"] = ec.MessageType
	ctx["raw_message"] = ec.RawMessage
	ctx["message_id"] = ec.MessageID
	ctx["self_id"] = ec.SelfID
	if len(ec.Message) > 0 {
		segs := make([]map[string]any, 0, len(ec.Message))
		for _, s := range ec.Message {
			segs = append(segs, map[string]any{
				"type": s.Type,
				"data": s.Data,
			})
		}
		ctx["segments"] = segs
	}
	return ctx
}

func (r *PythonRuntime) getProc(name string) *pythonproc.Proc {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.procs[name]
}

// Unload 卸载.
func (r *PythonRuntime) Unload(pluginName string) error {
	r.mu.Lock()
	proc, ok := r.procs[pluginName]
	if ok {
		delete(r.procs, pluginName)
		delete(r.loaded, pluginName)
	}
	r.mu.Unlock()
	if proc != nil {
		return proc.Stop()
	}
	return nil
}

// Reload 重启子进程.
func (r *PythonRuntime) Reload(ctx context.Context, host *Host, dir, entry string, meta *Metadata) error {
	_ = r.Unload(meta.Name)
	return r.Load(ctx, host, dir, entry, meta)
}

// Close 关闭所有子进程.
func (r *PythonRuntime) Close() error {
	r.mu.Lock()
	procs := make([]*pythonproc.Proc, 0, len(r.procs))
	for _, p := range r.procs {
		procs = append(procs, p)
	}
	r.procs = make(map[string]*pythonproc.Proc)
	r.mu.Unlock()

	for _, p := range procs {
		_ = p.Stop()
	}
	return nil
}

// 保留 stage1 协议结构.

type RPCMethod = string

type Envelope struct {
	Method RPCMethod       `json:"method"`
	ID     string          `json:"id,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type InitParams struct {
	PluginDir string `json:"plugin_dir"`
}

type ReadyResult struct {
	Plugins []PluginInfo `json:"plugins"`
}

type PluginInfo struct {
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	Description    string   `json:"description"`
	Commands       []string `json:"commands"`
	Permission     string   `json:"permission"`
	HasMessageHook bool     `json:"has_message_hook"`
	HasNoticeHook  bool     `json:"has_notice_hook"`
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

var _ = event.Any{}
var _ = errors.New

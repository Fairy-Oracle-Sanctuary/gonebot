// Lua 运行时实现.
package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"neobot/core/internal/permission"
)

// luaState 单个插件的 Lua VM.
type luaState struct {
	L          *lua.LState
	pluginName string
}

// LuaRuntime 每个插件一个独立 *lua.LState.
type LuaRuntime struct {
	mu     sync.Mutex
	states map[string]*luaState
	logger *slog.Logger
}

// NewLuaRuntime 创建 Lua 运行时.
func NewLuaRuntime() *LuaRuntime {
	return &LuaRuntime{
		states: make(map[string]*luaState),
		logger: slog.Default().With("module", "lua.runtime"),
	}
}

// Name 返回运行时名.
func (r *LuaRuntime) Name() string { return "lua" }

// Load 加载 Lua 插件.
func (r *LuaRuntime) Load(ctx context.Context, host *Host, dir, entry string, meta *Metadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 依赖校验 (lua: 检查 lib/ 下文件存在)
	if host != nil && host.Deps != nil && meta.Dependencies != nil {
		if err := host.Deps.ValidateLua(meta.Name, dir, meta.Dependencies.Lua); err != nil {
			return err
		}
	}

	if _, ok := r.states[meta.Name]; ok {
		r.unlocklessUnload(meta.Name)
	}

	L := lua.NewState()
	L.OpenLibs()

	state := &luaState{L: L, pluginName: meta.Name}
	r.states[meta.Name] = state

	injectSDK(L, host, state, meta)

	// 设置 package.path (含 [dependencies].local)
	if meta.Dependencies != nil && len(meta.Dependencies.Local) > 0 && host != nil && host.Deps != nil {
		paths := host.Deps.BuildLocalPaths(dir, meta.Dependencies.Local)
		L.SetGlobal("nb_package_paths", lua.LString(paths))
		r.logger.Debug("lua local paths set", "plugin", meta.Name, "paths", paths)
	}

	entryPath := filepath.Join(dir, entry)
	if _, err := os.Stat(entryPath); err != nil {
		L.Close()
		delete(r.states, meta.Name)
		return fmt.Errorf("entry not found: %s", entryPath)
	}
	if err := L.DoFile(entryPath); err != nil {
		L.Close()
		delete(r.states, meta.Name)
		return fmt.Errorf("exec %s: %w", entry, err)
	}

	r.logger.Info("plugin loaded", "plugin", meta.Name, "entry", entryPath)
	return nil
}

// Unload 卸载.
func (r *LuaRuntime) Unload(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.unlocklessUnload(name)
}

func (r *LuaRuntime) unlocklessUnload(name string) error {
	s, ok := r.states[name]
	if !ok {
		return nil
	}
	s.L.Close()
	delete(r.states, name)
	r.logger.Info("plugin unloaded", "plugin", name)
	return nil
}

// Reload 重新加载.
func (r *LuaRuntime) Reload(ctx context.Context, host *Host, dir, entry string, meta *Metadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unlocklessUnload(meta.Name)

	L := lua.NewState()
	L.OpenLibs()
	state := &luaState{L: L, pluginName: meta.Name}
	r.states[meta.Name] = state

	injectSDK(L, host, state, meta)

	entryPath := filepath.Join(dir, entry)
	if err := L.DoFile(entryPath); err != nil {
		L.Close()
		delete(r.states, meta.Name)
		return fmt.Errorf("exec %s: %w", entry, err)
	}
	r.logger.Info("plugin reloaded", "plugin", meta.Name)
	return nil
}

// Close 关闭所有 VM.
func (r *LuaRuntime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for n, s := range r.states {
		s.L.Close()
		delete(r.states, n)
	}
	return nil
}

// ---- Lua ↔ Go 互转工具 ----

func luaStr(v lua.LValue) string {
	if v.Type() == lua.LTNil {
		return ""
	}
	if s, ok := v.(lua.LString); ok {
		return string(s)
	}
	return v.String()
}

func goToLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case string:
		return lua.LString(x)
	case bool:
		return lua.LBool(x)
	case int:
		return lua.LNumber(x)
	case int64:
		return lua.LNumber(x)
	case float64:
		return lua.LNumber(x)
	default:
		return lua.LString(fmt.Sprintf("%v", x))
	}
}

func luaTableToMap(t *lua.LTable) map[string]any {
	out := make(map[string]any)
	t.ForEach(func(k, v lua.LValue) {
		key := luaStr(k)
		switch x := v.(type) {
		case lua.LString:
			out[key] = string(x)
		case lua.LNumber:
			out[key] = float64(x)
		case lua.LBool:
			out[key] = bool(x)
		case *lua.LTable:
			out[key] = luaTableToMap(x)
		}
	})
	return out
}

func parsePermissionName(s string) permission.Level {
	return permission.ParseLevelSimple(s)
}

package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"neobot/core/internal/plugin/runtime"
)

// Manager 插件管理器.
type Manager struct {
	mu sync.RWMutex

	logger   *slog.Logger
	registry *Registry
	host     *runtime.Host
	runtimes map[string]runtime.Runtime
	loaded   map[string]*runtime.LoadedPlugin

	pluginDirs []string // 扫描的目录列表 (按顺序)
}

// NewManager 创建管理器. 第一个目录是 Lua, 第二个 (可选) 是 Python.
func NewManager(luaDir, pythonDir string, registry *Registry, host *runtime.Host) *Manager {
	dirs := []string{}
	if luaDir != "" {
		dirs = append(dirs, luaDir)
	}
	if pythonDir != "" {
		dirs = append(dirs, pythonDir)
	}

	// 从 host.Deps 读取 venv 配置
	sharedVenv := ""
	useVenv := false
	if host != nil && host.Deps != nil {
		useVenv = host.Deps.IsUseVenv()
		sharedVenv = host.Deps.SharedVenvPath()
	}
	pyRT := runtime.NewPythonRuntime(runtime.PythonConfig{
		PythonBin:      "",
		ShimPath:       "shim/pyplugin_host.py",
		UseVenv:        useVenv,
		SharedVenvPath: sharedVenv,
	})
	return &Manager{
		logger:    slog.Default().With("module", "plugin.manager"),
		registry:  registry,
		host:      host,
		runtimes: map[string]runtime.Runtime{
			"lua":    runtime.NewLuaRuntime(),
			"python": pyRT,
		},
		loaded:     make(map[string]*runtime.LoadedPlugin),
		pluginDirs: dirs,
	}
}

// LoadAll 扫描并加载所有插件目录.
func (m *Manager) LoadAll(ctx context.Context) error {
	var lastErr error
	for _, dir := range m.pluginDirs {
		if _, err := os.Stat(dir); err != nil {
			m.logger.Debug("plugin dir not found", "dir", dir)
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			m.logger.Warn("read plugin dir failed", "dir", dir, "err", err.Error())
			lastErr = err
			continue
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "" || name[0] == '_' || name[0] == '.' {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)

		for _, n := range names {
			pluginDir := filepath.Join(dir, n)
			if err := m.LoadOne(ctx, pluginDir); err != nil {
				m.logger.Warn("load plugin failed", "plugin", n, "err", err.Error())
				lastErr = err
			}
		}
	}
	return lastErr
}

// LoadOne 加载单个插件.
func (m *Manager) LoadOne(ctx context.Context, dir string) error {
	md, entry, err := LoadMetadata(dir)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	rt, ok := m.runtimes[md.Runtime]
	if !ok {
		return fmt.Errorf("unsupported runtime: %s", md.Runtime)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.registry.UnregisterByPlugin(md.Name)
	if old, ok := m.loaded[md.Name]; ok {
		_ = old
		_ = m.runtimes[md.Runtime].Unload(md.Name)
	}

	rmd := toRuntimeMeta(md)
	if err := rt.Load(ctx, m.host, dir, entry, rmd); err != nil {
		return fmt.Errorf("runtime load: %w", err)
	}

	m.registry.SetPluginMeta(&PluginMeta{
		Name: md.Name, Description: md.Description, Usage: md.Usage,
	})

	m.loaded[md.Name] = &runtime.LoadedPlugin{
		Metadata: *toRuntimeMeta(md), Dir: dir, Entry: entry,
	}
	m.logger.Info("plugin ready", "plugin", md.Name, "runtime", md.Runtime)
	return nil
}

// Reload 重载单个插件.
func (m *Manager) Reload(ctx context.Context, name string) error {
	m.mu.RLock()
	lp, ok := m.loaded[name]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("plugin not loaded: %s", name)
	}
	return m.LoadOne(ctx, lp.Dir)
}

// Unload 卸载单个插件.
func (m *Manager) Unload(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.loaded[name]; !ok {
		return nil
	}
	m.registry.UnregisterByPlugin(name)
	for _, rt := range m.runtimes {
		_ = rt.Unload(name)
	}
	delete(m.loaded, name)
	m.logger.Info("plugin unloaded", "plugin", name)
	return nil
}

// Loaded 列出已加载插件名.
func (m *Manager) Loaded() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.loaded))
	for n := range m.loaded {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// PluginDir 返回第一个插件目录 (兼容旧 API).
func (m *Manager) PluginDir() string {
	if len(m.pluginDirs) == 0 {
		return ""
	}
	return m.pluginDirs[0]
}

// PluginDirs 返回所有插件目录.
func (m *Manager) PluginDirs() []string {
	return append([]string{}, m.pluginDirs...)
}

// Close 关闭所有运行时.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, rt := range m.runtimes {
		_ = rt.Close()
		m.logger.Info("runtime closed", "name", name)
	}
	return nil
}

func toRuntimeMeta(md *Metadata) *runtime.Metadata {
	rmd := &runtime.Metadata{
		Name:        md.Name,
		Version:     md.Version,
		Author:      md.Author,
		Description: md.Description,
		Usage:       md.Usage,
		Permission:  md.Permission,
		Tags:        md.Tags,
		Runtime:     md.Runtime,
		Config:      md.Config,
	}
	if md.Dependencies != nil {
		rmd.Dependencies = &runtime.Dependency{
			Python: md.Dependencies.Python,
			Lua:    md.Dependencies.Lua,
			Local:  md.Dependencies.Local,
		}
	}
	return rmd
}
// Package deps 插件依赖管理 (阶段 1).
//
// 职责:
//   - Python: 从 plugin.toml 的 [dependencies].python 或 requirements.txt
//     读取 pip 包列表, 调用 pip install 安装
//   - Lua: 校验 [dependencies].lua 声明的库在 plugins/<name>/lib/ 下存在
//   - 缓存已安装包, 避免重复安装
package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"neobot/core/internal/logger"
)

// Config 依赖管理配置.
type Config struct {
	PythonBin      string    // python 可执行路径
	PipIndex       string    // pip 源 URL (可选, 默认 pypi)
	PipExtraArgs   []string  // 额外 pip 参数
	Enabled        bool      // 是否启用依赖管理
	CacheDir       string    // 依赖缓存目录 (默认 ./data/plugin_deps)
	Timeout        int       // 安装超时秒
	UseVenv        bool      // 阶段 2: 启用 venv 隔离
	SharedVenvPath string    // 阶段 2: 共享 venv 路径 (留空=每插件独立)
}

// Manager 依赖管理器.
type Manager struct {
	cfg    Config
	logger *slog.Logger
	mu     sync.Mutex

	useVenv    bool   // 阶段 2: 是否启用 venv 隔离
	sharedVenv string // 阶段 2: 共享 venv 路径 (留空=每插件独立)

	// 已安装包缓存: <runtime>:<name>:<pkg> -> version
	cache map[string]string
}

// New 创建依赖管理器.
func New(cfg Config) *Manager {
	if cfg.PythonBin == "" {
		cfg.PythonBin = "python"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = "./data/plugin_deps"
	}
	// 转绝对路径, 避免 cmd.Dir 改变后相对路径失效
	if !filepath.IsAbs(cfg.CacheDir) {
		if abs, err := filepath.Abs(cfg.CacheDir); err == nil {
			cfg.CacheDir = abs
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 300
	}
	if !cfg.Enabled {
		cfg.Enabled = true
	}
	_ = os.MkdirAll(cfg.CacheDir, 0o755)

	// 共享 venv 路径转绝对
	shared := cfg.SharedVenvPath
	if shared != "" && !filepath.IsAbs(shared) {
		if abs, err := filepath.Abs(shared); err == nil {
			shared = abs
		}
	}

	return &Manager{
		cfg:        cfg,
		logger:     logger.Module("deps"),
		cache:      make(map[string]string),
		useVenv:    cfg.UseVenv,
		sharedVenv: shared,
	}
}

// IsUseVenv 返回是否启用 venv 隔离.
func (m *Manager) IsUseVenv() bool { return m.useVenv }

// SetUseVenv 动态启用/禁用 venv 隔离.
func (m *Manager) SetUseVenv(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.useVenv = v
}

// SharedVenvPath 返回共享 venv 路径.
func (m *Manager) SharedVenvPath() string { return m.sharedVenv }

// SetSharedVenvPath 设置共享 venv 路径.
func (m *Manager) SetSharedVenvPath(p string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p != "" && !filepath.IsAbs(p) {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	m.sharedVenv = p
}

// InstallPython 安装 Python 依赖.
//
// 来源: (1) requirements.txt (2) plugin.toml [dependencies].python
// 重复运行是幂等的: 已安装的包会被 pip 跳过 (除非版本不匹配).
func (m *Manager) InstallPython(pluginName, pluginDir string, declared []string) error {
	if !m.cfg.Enabled {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// 收集所有需求
	reqs, source, err := m.collectPythonReqs(pluginDir, declared)
	if err != nil {
		return fmt.Errorf("collect python reqs: %w", err)
	}
	if len(reqs) == 0 {
		m.logger.Debug("no python deps", "plugin", pluginName)
		return nil
	}

	// 写入临时 requirements.txt
	reqFile := filepath.Join(m.cfg.CacheDir, fmt.Sprintf("%s.requirements.txt", sanitizeName(pluginName)))
	if err := os.WriteFile(reqFile, []byte(strings.Join(reqs, "\n")+"\n"), 0o644); err != nil {
		return fmt.Errorf("write req file: %w", err)
	}

	// 检查是否需要安装 (基于 hash 缓存)
	hash := hashReqs(reqs)
	cacheKey := fmt.Sprintf("py:%s:%s", pluginName, hash)
	if cached, ok := m.loadCache(); ok {
		if cached[cacheKey] == "ok" {
			m.logger.Debug("python deps already installed", "plugin", pluginName, "source", source)
			return nil
		}
	}

	m.logger.Info("installing python deps",
		"plugin", pluginName, "count", len(reqs), "source", source,
		"reqs", reqs)

	// 执行 pip install
	args := []string{"-m", "pip", "install",
		"--disable-pip-version-check",
		"--no-warn-script-location",
		"--user",
	}
	if m.cfg.PipIndex != "" {
		args = append(args, "-i", m.cfg.PipIndex)
	}
	args = append(args, m.cfg.PipExtraArgs...)
	args = append(args, "-r", reqFile)

	cmd := exec.Command(m.cfg.PythonBin, args...)
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), "PIP_DISABLE_PIP_VERSION_CHECK=1")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("pip install failed: %w\noutput: %s", err, string(output))
	}

	m.logger.Info("python deps installed",
		"plugin", pluginName, "count", len(reqs))

	// 更新缓存
	m.cache[cacheKey] = "ok"
	_ = m.saveCache()
	return nil
}

// ValidateLua 校验 Lua 依赖.
// 阶段 1 只检查文件存在性, 不自动下载.
func (m *Manager) ValidateLua(pluginName, pluginDir string, declared []string) error {
	if !m.cfg.Enabled {
		return nil
	}
	if len(declared) == 0 {
		return nil
	}

	libDir := filepath.Join(pluginDir, "lib")
	var missing []string

	for _, libName := range declared {
		// 支持 libName.lua 或 libName/init.lua
		flat := filepath.Join(libDir, libName+".lua")
		mod := filepath.Join(libDir, libName, "init.lua")
		if !fileExists(flat) && !fileExists(mod) {
			missing = append(missing, libName)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("lua libs missing for plugin %s: %v (expected in %s)",
			pluginName, missing, libDir)
	}

	m.logger.Debug("lua deps ok", "plugin", pluginName, "count", len(declared))
	return nil
}

// SetupLocalPaths 生成 Lua package.path 设置脚本 (供 plugin.lua 头部 require).
//
// 插件中可这样用:
//     package.path = require("neobot.deps").paths .. "?.lua;" .. package.path
func (m *Manager) BuildLocalPaths(pluginDir string, locals []string) string {
	var paths []string
	for _, p := range locals {
		abs := filepath.Join(pluginDir, p)
		// Lua require 路径分隔符是 '/', Windows 也要用 '/'
		abs = filepath.ToSlash(abs)
		// 末尾添加 '/?.lua' 或 '/?/init.lua'
		paths = append(paths, abs+"/?.lua")
		paths = append(paths, abs+"/?/init.lua")
	}
	return strings.Join(paths, ";")
}

// ---- 内部辅助 ----

// collectPythonReqs 收集 Python 需求 (合并 requirements.txt 和 plugin.toml).
func (m *Manager) collectPythonReqs(pluginDir string, declared []string) ([]string, string, error) {
	var reqs []string
	var source []string

	// 1. requirements.txt
	reqFile := filepath.Join(pluginDir, "requirements.txt")
	if data, err := os.ReadFile(reqFile); err == nil {
		for _, line := range splitLines(string(data)) {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			reqs = append(reqs, line)
		}
		source = append(source, "requirements.txt")
	}

	// 2. plugin.toml [dependencies].python
	for _, r := range declared {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		reqs = append(reqs, r)
		source = append(source, "plugin.toml")
	}

	return reqs, strings.Join(source, "+"), nil
}

func splitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func sanitizeName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

func hashReqs(reqs []string) string {
	h := sha256.New()
	for _, r := range reqs {
		h.Write([]byte(r))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ---- 缓存持久化 ----

type cacheFile struct {
	Updated time.Time          `json:"updated"`
	Entries map[string]string  `json:"entries"`
}

func (m *Manager) cachePath() string {
	return filepath.Join(m.cfg.CacheDir, "deps_cache.json")
}

func (m *Manager) loadCache() (map[string]string, bool) {
	data, err := os.ReadFile(m.cachePath())
	if err != nil {
		return nil, false
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, false
	}
	return cf.Entries, true
}

func (m *Manager) saveCache() error {
	cf := cacheFile{Updated: time.Now(), Entries: m.cache}
	data, err := json.Marshal(cf)
	if err != nil {
		return err
	}
	return os.WriteFile(m.cachePath(), data, 0o644)
}
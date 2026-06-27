// Package initfiles 首次运行时初始化必要文件和目录.
//
// 当二进制在空目录首次运行时, 自动创建:
//   - config.toml             (默认配置)
//   - plugins_lua/            (含示例插件)
//   - plugins_py/             (含示例插件)
//   - shim/                   (Python shim + SDK)
//   - data/, data/plugin_deps/
package initfiles

import (
	"fmt"
	"neobot/core/internal/logx"
	"os"
	"path/filepath"
)

// Logger 初始化日志输出接口.
var Logger func(format string, args ...interface{})

func logf(format string, args ...interface{}) {
	if Logger != nil {
		Logger(format, args...)
	}
}

// File 代表一个要写入的文件.
type File struct {
	Path    string // 相对于工作目录的路径
	Content string // 文件内容
	Dir     bool   // 是否为目录
}

// All 返回所有需要初始化的文件和目录.
func All(workDir string) []File {
	// 目录
	dirs := []string{
		"data",
		"data/plugin_deps",
		"plugins_lua",
		"plugins_py",
		"shim",
		"shim/neobot_sdk",
		"plugins_lua/echo",
		"plugins_lua/hello",
		"plugins_lua/greeter",
		"plugins_lua/greeter/lib",
		"plugins_lua/broadcast",
		"plugins_py/echo",
		"plugins_py/webfetch",
	}

	var out []File
	for _, d := range dirs {
		out = append(out, File{Path: filepath.Join(workDir, d), Dir: true})
	}

	// ---- shim 文件 ----
	shimFiles := []File{
		{Path: filepath.Join(workDir, "shim/pyplugin_host.py"), Content: shimPyPluginHost},
		{Path: filepath.Join(workDir, "shim/neobot_sdk/__init__.py"), Content: shimSDKInit},
		{Path: filepath.Join(workDir, "shim/neobot_sdk/plugin.py"), Content: shimSDKPlugin},
	}
	out = append(out, shimFiles...)

	// ---- Lua 示例插件 ----
	luaFiles := []File{
		{Path: filepath.Join(workDir, "plugins_lua/echo/plugin.toml"), Content: luaEchoToml},
		{Path: filepath.Join(workDir, "plugins_lua/echo/plugin.lua"), Content: luaEchoLua},
		{Path: filepath.Join(workDir, "plugins_lua/hello/plugin.toml"), Content: luaHelloToml},
		{Path: filepath.Join(workDir, "plugins_lua/hello/plugin.lua"), Content: luaHelloLua},
		{Path: filepath.Join(workDir, "plugins_lua/greeter/plugin.toml"), Content: luaGreeterToml},
		{Path: filepath.Join(workDir, "plugins_lua/greeter/plugin.lua"), Content: luaGreeterLua},
		{Path: filepath.Join(workDir, "plugins_lua/greeter/lib/string-utils.lua"), Content: luaGreeterLib},
		{Path: filepath.Join(workDir, "plugins_lua/broadcast/plugin.toml"), Content: luaBroadcastToml},
		{Path: filepath.Join(workDir, "plugins_lua/broadcast/plugin.lua"), Content: luaBroadcastLua},
	}
	out = append(out, luaFiles...)

	// ---- Python 示例插件 ----
	pyFiles := []File{
		{Path: filepath.Join(workDir, "plugins_py/echo/plugin.toml"), Content: pyEchoToml},
		{Path: filepath.Join(workDir, "plugins_py/echo/plugin.py"), Content: pyEchoPy},
		{Path: filepath.Join(workDir, "plugins_py/webfetch/plugin.toml"), Content: pyWebfetchToml},
		{Path: filepath.Join(workDir, "plugins_py/webfetch/plugin.py"), Content: pyWebfetchPy},
	}
	out = append(out, pyFiles...)

	return out
}

// Ensure 确保所有初始文件存在, 不存在则创建, 已存在则跳过.
func Ensure(workDir string) error {
	created, skipped := 0, 0
	for _, f := range All(workDir) {
		if f.Dir {
			if err := os.MkdirAll(f.Path, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", f.Path, err)
			}
			logf("[init] %s %s", logx.T("mkdir"), f.Path)
			created++
			continue
		}
		// 文件已存在则跳过
		if _, err := os.Stat(f.Path); err == nil {
			skipped++
			continue
		}
		// 确保父目录存在
		parent := filepath.Dir(f.Path)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("mkdir parent %s: %w", parent, err)
		}
		if err := os.WriteFile(f.Path, []byte(f.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
		logf("[init] %s %s", logx.T("write"), f.Path)
		created++
	}
	logf("[init] %s: %d created, %d skipped (already exist)", logx.T("done"), created, skipped)
	return nil
}

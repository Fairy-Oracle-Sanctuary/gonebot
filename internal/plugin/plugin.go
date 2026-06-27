// Package plugin 插件系统核心.
//
//   - plugin.go: 元信息 (Metadata) 与展示信息 (PluginMeta)
//   - registry.go: 命令与事件注册表
//   - manager.go: 加载/卸载/重载
//   - watcher.go: 文件热重载
//   - router.go: 事件分发
//   - runtime/: 运行时抽象与 LuaRuntime
package plugin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"neobot/core/internal/permission"
)

// Metadata 来自 plugin.toml 的元信息.
type Metadata struct {
	Name         string           `toml:"name"`
	Version      string           `toml:"version"`
	Author       string           `toml:"author"`
	Description  string           `toml:"description"`
	Usage        string           `toml:"usage"`
	Permission   string           `toml:"permission"`
	Tags         []string         `toml:"tags"`
	Runtime      string           `toml:"runtime"`
	Config       map[string]any   `toml:"config"`
	Dependencies *Dependencies    `toml:"dependencies,omitempty"`
	Extra        map[string]any   `toml:"-"`
}

// Dependencies 依赖声明 (阶段 1).
type Dependencies struct {
	// Python pip 包列表 (等价于 requirements.txt 单行格式).
	Python []string `toml:"python,omitempty"`
	// Lua 库名列表 (校验 plugins/<name>/lib/<name>.lua 是否存在).
	Lua []string `toml:"lua,omitempty"`
	// 本地相对路径 (加载时设置 package.path).
	Local []string `toml:"local,omitempty"`
}

// RequiredLevel 返回命令所需权限.
func (m *Metadata) RequiredLevel() permission.Level {
	lvl, _ := permission.ParseLevel(m.Permission)
	return lvl
}

// PluginMeta 展示用元信息.
type PluginMeta struct {
	Name        string
	Description string
	Usage       string
}

// LoadedPlugin 已加载的插件.
type LoadedPlugin struct {
	Metadata Metadata
	Dir      string
	Entry    string
}

// LoadMetadata 读取 plugin.toml.
func LoadMetadata(dir string) (*Metadata, string, error) {
	path := filepath.Join(dir, "plugin.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read manifest: %w", err)
	}

	md := &Metadata{Extra: map[string]any{}}
	if err := toml.Unmarshal(data, md); err != nil {
		return nil, "", fmt.Errorf("parse manifest: %w", err)
	}

	if md.Name == "" {
		return nil, "", errors.New("plugin name is required")
	}
	if md.Runtime == "" {
		md.Runtime = "lua"
	}

	entry := "plugin.lua"
	if md.Runtime == "python" {
		entry = "plugin.py"
	}
	return md, entry, nil
}
// Package runtime: 插件元信息 (与 plugin.Metadata 独立, 避免循环依赖).
package runtime

// Dependency 依赖声明.
type Dependency struct {
	// Python: pip 包列表
	Python []string
	// Lua: 库名列表 (校验 plugins/<name>/lib/<name>.lua)
	Lua []string
	// Local: 本地相对路径
	Local []string
}

// Metadata 简化的元信息, 来自 LoadMetadata.
type Metadata struct {
	Name         string
	Version      string
	Author       string
	Description  string
	Usage        string
	Permission   string
	Tags         []string
	Runtime      string
	Config       map[string]any
	Dependencies *Dependency
}

// LoadedPlugin 已加载的插件实例.
type LoadedPlugin struct {
	Metadata Metadata
	Dir      string
	Entry    string
}

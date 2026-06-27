// Package config 加载 TOML 配置.
package config

import (
	"errors"
	"fmt"
	"neobot/core/internal/logx"
	"os"
	"path/filepath"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

// Config 是整个应用的根配置.
type Config struct {
	Bot       BotConfig       `toml:"bot"`
	NapCatWS  NapCatWSConfig  `toml:"napcat_ws"`
	ReverseWS ReverseWSConfig `toml:"reverse_ws"`
	Redis     RedisConfig     `toml:"redis"`
	MySQL     MySQLConfig     `toml:"mysql"`
	Logging   LoggingConfig   `toml:"logging"`
	Plugins   PluginsConfig   `toml:"plugins"`
	Browser   BrowserConfig   `toml:"browser"`
	Image     ImageConfig     `toml:"image"`
}

// BotConfig 机器人自身配置.
type BotConfig struct {
	SelfID            int64    `toml:"self_id"`
	Nickname          string   `toml:"nickname"`
	CommandPrefix     []string `toml:"command_prefix"`
	IgnoreSelfMessage bool     `toml:"ignore_self_message"`
	SuperUsers        []int64  `toml:"superusers"`
	AdminGroups       []int64  `toml:"admin_groups"`
}

// NapCatWSConfig 正向 WebSocket.
type NapCatWSConfig struct {
	URI               string `toml:"uri"`
	Token             string `toml:"token"`
	ReconnectInterval int    `toml:"reconnect_interval"`
	HeartbeatInterval int    `toml:"heartbeat_interval"`
	APIRequestTimeout int    `toml:"api_request_timeout"`
}

// ReverseWSConfig 反向 WebSocket.
type ReverseWSConfig struct {
	Enabled bool   `toml:"enabled"`
	Host    string `toml:"host"`
	Port    int    `toml:"port"`
	Token   string `toml:"token"`
}

// RedisConfig Redis 连接.
type RedisConfig struct {
	Addr     string `toml:"addr"`
	Password string `toml:"password"`
	DB       int    `toml:"db"`
	PoolSize int    `toml:"pool_size"`
}

// MySQLConfig MySQL 连接.
type MySQLConfig struct {
	DSN     string `toml:"dsn"`
	MaxOpen int    `toml:"max_open"`
	MaxIdle int    `toml:"max_idle"`
	MaxLife int    `toml:"max_life_minutes"`
}

// LoggingConfig 日志.
type LoggingConfig struct {
	Level   string `toml:"level"`
	Output  string `toml:"output"`
	File    string `toml:"file"`
	MaxSize int    `toml:"max_size_mb"`
	MaxKeep int    `toml:"max_backups"`
}

// PluginsConfig 插件相关.
type PluginsConfig struct {
	Lua    LuaPluginConfig    `toml:"lua"`
	Python PythonPluginConfig `toml:"python"`
	Deps   DepsConfig         `toml:"deps"`
}

// DepsConfig 依赖管理配置 (阶段 1+2).
type DepsConfig struct {
	Enabled        bool     `toml:"enabled"`
	PythonBin      string   `toml:"python_bin"`
	PipIndex       string   `toml:"pip_index"`
	PipExtraArgs   []string `toml:"pip_extra_args"`
	CacheDir       string   `toml:"cache_dir"`
	Timeout        int      `toml:"timeout_sec"`
	UseVenv        bool     `toml:"use_venv"`         // 阶段 2: 是否启用 venv
	SharedVenvPath string   `toml:"shared_venv_path"` // 阶段 2: 共享 venv 路径 (留空=每插件独立)
}

// PythonPluginConfig Python 插件配置.
type PythonPluginConfig struct {
	Enabled   bool   `toml:"enabled"`
	Dir       string `toml:"dir"`
	PythonBin string `toml:"python_bin"`
	ShimPath  string `toml:"shim_path"`
}

// BrowserConfig 浏览器配置.
type BrowserConfig struct {
	Enabled  bool   `toml:"enabled"`
	PoolSize int    `toml:"pool_size"`
	Path     string `toml:"chromium_path"`
	Headless bool   `toml:"headless"`
}

// ImageConfig 图片渲染配置.
type ImageConfig struct {
	Width   int `toml:"width"`
	Height  int `toml:"height"`
	Quality int `toml:"quality"`
	Timeout int `toml:"timeout_sec"`
}

// LuaPluginConfig Lua 插件配置.
type LuaPluginConfig struct {
	Enabled   bool   `toml:"enabled"`
	Dir       string `toml:"dir"`
	MaxMemMB  int    `toml:"max_memory_mb"`
	MaxCPUSec int    `toml:"max_cpu_sec"`
}

// Default 返回带默认值的配置.
func Default() *Config {
	return &Config{
		Bot: BotConfig{
			CommandPrefix:     []string{"/", "!", "＃"},
			IgnoreSelfMessage: true,
		},
		NapCatWS: NapCatWSConfig{
			URI:               "ws://127.0.0.1:30001",
			ReconnectInterval: 5,
			HeartbeatInterval: 30,
			APIRequestTimeout: 30,
		},
		ReverseWS: ReverseWSConfig{
			Enabled: false,
			Host:    "0.0.0.0",
			Port:    8080,
		},
		Redis: RedisConfig{
			Addr:     "127.0.0.1:6379",
			PoolSize: 20,
		},
		MySQL: MySQLConfig{
			MaxOpen: 20,
			MaxIdle: 5,
			MaxLife: 30,
		},
		Logging: LoggingConfig{
			Level:   "info",
			Output:  "stdout",
			MaxSize: 100,
			MaxKeep: 7,
		},
		Browser: BrowserConfig{
			Enabled:  false,
			PoolSize: 3,
			Headless: true,
		},
		Image: ImageConfig{
			Width:   800,
			Quality: 90,
			Timeout: 30,
		},
		Plugins: PluginsConfig{
			Lua: LuaPluginConfig{
				Enabled:   true,
				Dir:       "plugins_lua",
				MaxMemMB:  64,
				MaxCPUSec: 5,
			},
			Python: PythonPluginConfig{
				Enabled:   false,
				Dir:       "plugins_py",
				PythonBin: "python3",
				ShimPath:  "shim/pyplugin_host.py",
			},
			Deps: DepsConfig{
				Enabled:        true,
				PythonBin:      "python3",
				CacheDir:       "./data/plugin_deps",
				Timeout:        300,
				UseVenv:        true,               // 阶段 2 默认开启
				SharedVenvPath: "plugins_py/.venv", // 默认共享 venv
			},
		},
	}
}

var (
	mu       sync.RWMutex
	instance *Config
)

// Load 从指定路径加载配置.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		path = "config.toml"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logx.Printf("[INFO] %s\n", logx.T("config not found, writing defaults")+": "+abs)
			if werr := writeDefault(abs, cfg); werr != nil {
				logx.Printf("[WARN] %s: %v\n", logx.T("failed to write default config"), werr)
			}
			applyEnv(cfg)
			setGlobal(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	applyEnv(cfg)
	setGlobal(cfg)
	return cfg, nil
}

// Global 返回全局配置.
func Global() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

func setGlobal(c *Config) {
	mu.Lock()
	defer mu.Unlock()
	instance = c
}

func (c *Config) validate() error {
	if c.NapCatWS.URI == "" {
		return errors.New("napcat_ws.uri is required")
	}
	if c.NapCatWS.ReconnectInterval <= 0 {
		c.NapCatWS.ReconnectInterval = 5
	}
	if c.NapCatWS.APIRequestTimeout <= 0 {
		c.NapCatWS.APIRequestTimeout = 30
	}
	if len(c.Bot.CommandPrefix) == 0 {
		c.Bot.CommandPrefix = []string{"/"}
	}
	return nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("NEOTOT_NAPCAT_URI"); v != "" {
		c.NapCatWS.URI = v
	}
	if v := os.Getenv("NEOTOT_NAPCAT_TOKEN"); v != "" {
		c.NapCatWS.Token = v
	}
	if v := os.Getenv("NEOTOT_LOG_LEVEL"); v != "" {
		c.Logging.Level = v
	}
}

// writeDefault 将默认配置写入 TOML 文件.
func writeDefault(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	// 添加注释头
	header := []byte("# NeoBot 配置文件\n# 修改后重启生效\n\n")
	data = append(header, data...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	logx.Printf("[INFO] %s\n", logx.T("default config written to")+": "+path)
	return nil
}

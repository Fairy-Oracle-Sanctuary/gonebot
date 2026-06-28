package logx

// zhMessages 英文日志消息 → 中文翻译映射表.
// slog.Handler 在输出时自动查表替换.
var zhMessages = map[string]string{
	// === cmd/neobot/main.go ===
	"neobot-go starting":             "NeoBot 正在启动",
	"received signal, shutting down": "收到退出信号, 正在关闭",
	"redis connect failed":           "Redis 连接失败",
	"redis connected":                "Redis 已连接",
	"mysql connect failed":           "MySQL 连接失败",
	"mysql connected":                "MySQL 已连接",
	"browser init failed":            "浏览器初始化失败",
	"browser pool ready":             "浏览器池就绪",
	"reversews exited":               "反向 WS 服务已退出",
	"some plugins failed to load":    "部分插件加载失败",
	"plugins loaded":                 "插件已加载",
	"watcher exited":                 "文件监控已退出",
	"connecting to napcat":           "正在连接 NapCat",
	"connect exited":                 "连接异常退出",
	"closing":                        "正在关闭",
	"neobot-go exited cleanly":       "NeoBot 已正常退出",
	// 启动前 stderr 输出 (logx.T):
	"load config":  "加载配置",
	"init files":   "初始化文件",
	"setup logger": "初始化日志",

	// === internal/ws/client.go ===
	"ws disconnected":     "WS 连接断开",
	"ws connected":        "WS 已连接",
	"decode probe failed": "心跳解析失败",
	"decode event failed": "事件解析失败",

	// === internal/reversews/server.go ===
	"reversews listening":    "反向 WS 正在监听",
	"ws upgrade failed":      "WS 升级失败",
	"ws client connected":    "WS 客户端已连接",
	"ws client disconnected": "WS 客户端已断开",
	"ws read error":          "WS 读取错误",
	"parse event failed":     "事件解析失败",
	"ws write error":         "WS 写入错误",

	// === internal/plugin/manager.go ===
	"plugin dir not found":   "插件目录未找到",
	"read plugin dir failed": "读取插件目录失败",
	"load plugin failed":     "加载插件失败",
	"plugin ready":           "插件就绪",
	"plugin unloaded":        "插件已卸载",
	"runtime closed":         "运行时已关闭",

	// === internal/plugin/pythonproc/proc.go ===
	"proc started":                  "子进程已启动",
	"read error":                    "读取错误",
	"subprocess exited":             "子进程已退出",
	"decode msg failed":             "消息解析失败",
	"plugin ready (ready received)": "插件就绪 (已收到 ready)",
	"call cmd failed":               "命令调用失败",

	// === internal/plugin/deps/venv.go ===
	"venv already up-to-date":                        "venv 已是最新",
	"creating venv":                                  "正在创建 venv",
	"save venv info failed":                          "保存 venv 信息失败",
	"venv updated":                                   "venv 已更新",
	"creating venv with uv":                          "使用 uv 创建 venv",
	"uv venv failed, falling back to python -m venv": "uv venv 失败, 回退到 python -m venv",
	"creating venv with python -m venv":              "使用 python -m venv 创建 venv",
	"uv pip install failed, falling back to pip":     "uv pip install 失败, 回退到 pip",

	// === internal/plugin/deps/manager.go ===
	"no python deps":                "无 Python 依赖",
	"python deps already installed": "Python 依赖已安装",
	"installing python deps":        "正在安装 Python 依赖",
	"python deps installed":         "Python 依赖已安装完成",
	"lua deps ok":                   "Lua 依赖校验通过",

	// === internal/plugin/runtime/lua.go ===
	"lua local paths set": "Lua 本地路径已设置",
	"plugin reloaded":     "插件已重载",

	// === internal/plugin/runtime/python.go ===
	"plugin ready (isolated proc)": "插件就绪 (独立进程)",

	// === internal/plugin/runtime/lua_sdk.go ===
	"lua call failed": "Lua 调用失败",

	// === internal/plugin/router.go ===
	"permission denied":             "权限不足",
	"command handler returned nil":  "命令处理器返回空",
	"command not found in registry": "命令未注册",
	"dispatch message":              "分发消息",
	"dispatch notice":               "分发通知",
	"command matched":               "匹配命令",
	"replying":                      "回复消息",

	// === internal/plugin/watcher.go ===
	"watcher started":         "文件监控已启动",
	"file changed, reloading": "文件已变更, 正在重载",
	"reload failed":           "重载失败",
	"watcher error":           "文件监控错误",

	// === internal/service/browser/pool.go ===
	"browser launched":     "浏览器已启动",
	"browser close failed": "浏览器关闭失败",
	"browser pool closed":  "浏览器池已关闭",

	// === internal/service/image/renderer.go ===
	"wait load failed": "页面加载等待超时",

	// === internal/config/config.go (通过 logx.Printf/logx.T 输出) ===
	"config not found, writing defaults": "配置文件未找到, 正在写入默认配置",
	"failed to write default config":     "写入默认配置失败",
	"default config written to":          "默认配置已写入",

	// === internal/initfiles/initfiles.go (通过 logx.T) ===
	"mkdir": "创建目录",
	"write": "写入文件",
	"done":  "完成",
}

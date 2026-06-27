// neobot 是 NeoBot Go 版的核心入口.
//
// 集成: WS 客户端 / ReverseWS 服务端 / Lua 插件系统 / Redis / MySQL / BrowserPool / Renderer.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"neobot/core/internal/bot"
	"neobot/core/internal/config"
	"neobot/core/internal/event"
	"neobot/core/internal/initfiles"
	"neobot/core/internal/logger"
	"neobot/core/internal/logx"
	"neobot/core/internal/permission"
	"neobot/core/internal/plugin"
	"neobot/core/internal/plugin/deps"
	"neobot/core/internal/plugin/runtime"
	"neobot/core/internal/reversews"
	"neobot/core/internal/service/browser"
	"neobot/core/internal/service/image"
	"neobot/core/internal/service/mysql"
	"neobot/core/internal/service/redis"
	"neobot/core/internal/ws"
)

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logx.Printf(logx.T("load config")+": %v\n", err)
		os.Exit(1)
	}

	// 首次运行: 创建必要的目录和示例文件
	initfiles.Logger = func(format string, args ...interface{}) {
		logx.Printf(format+"\n", args...)
	}
	workDir, _ := os.Getwd()
	if err := initfiles.Ensure(workDir); err != nil {
		logx.Printf("[WARN] %s: %v\n", logx.T("init files"), err)
	}

	if err := logger.Setup(cfg.Logging.Level, cfg.Logging.Output, cfg.Logging.File); err != nil {
		logx.Printf(logx.T("setup logger")+": %v\n", err)
		os.Exit(1)
	}
	log := logger.Module("main")
	log.Info("neobot-go starting", "version", "0.4.0", "config", *cfgPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info("received signal, shutting down", "signal", sig.String())
		cancel()
	}()

	// ---- 服务装配 ----
	var redisSvc runtime.RedisService
	var mysqlSvc runtime.MySQLService
	var renderer runtime.RendererService
	var browserPool *browser.Pool

	// Redis
	if cfg.Redis.Addr != "" {
		rs, err := redis.New(redis.Config{
			Addr: cfg.Redis.Addr, Password: cfg.Redis.Password,
			DB: cfg.Redis.DB, PoolSize: cfg.Redis.PoolSize,
		})
		if err != nil {
			log.Warn("redis connect failed", "err", err.Error())
		} else {
			redisSvc = rs
			log.Info("redis connected", "addr", cfg.Redis.Addr)
		}
	}

	// MySQL
	if cfg.MySQL.DSN != "" {
		ms, err := mysql.New(mysql.Config{
			DSN: cfg.MySQL.DSN, MaxOpen: cfg.MySQL.MaxOpen,
			MaxIdle: cfg.MySQL.MaxIdle, MaxLife: cfg.MySQL.MaxLife,
		})
		if err != nil {
			log.Warn("mysql connect failed", "err", err.Error())
		} else {
			mysqlSvc = ms
			log.Info("mysql connected")
		}
	}

	// Browser + Renderer
	if cfg.Browser.Enabled {
		pool, err := browser.New(browser.Config{
			Enabled:  true,
			PoolSize: 3,
		})
		if err != nil {
			log.Warn("browser init failed", "err", err.Error())
		} else {
			browserPool = pool
			renderer = image.New(pool)
			log.Info("browser pool ready")
		}
	}

	// ---- WS 客户端 (正向连接 NapCat) ----
	wsc := ws.NewClient(ws.Options{
		URL:               cfg.NapCatWS.URI,
		Token:             cfg.NapCatWS.Token,
		ReconnectInterval: cfg.NapCatWS.ReconnectInterval,
		APIRequestTimeout: cfg.NapCatWS.APIRequestTimeout,
	})
	b := bot.New(wsc)

	// ---- ReverseWS 服务端 (可选) ----
	var revSrv *reversews.Server
	if cfg.ReverseWS.Enabled {
		revSrv = reversews.New(reversews.Config{
			Addr:  fmt.Sprintf("%s:%d", cfg.ReverseWS.Host, cfg.ReverseWS.Port),
			Token: cfg.ReverseWS.Token,
		}, nil)
		go func() {
			if err := revSrv.Start(ctx); err != nil {
				log.Warn("reversews exited", "err", err.Error())
			}
		}()
	}

	// ---- 权限 ----
	perm := permission.NewChecker(cfg.Bot.SuperUsers, cfg.Bot.AdminGroups)

	// ---- 依赖管理 (阶段 1+2) ----
	depsMgr := deps.New(deps.Config{
		Enabled:        cfg.Plugins.Deps.Enabled,
		PythonBin:      cfg.Plugins.Deps.PythonBin,
		PipIndex:       cfg.Plugins.Deps.PipIndex,
		PipExtraArgs:   cfg.Plugins.Deps.PipExtraArgs,
		CacheDir:       cfg.Plugins.Deps.CacheDir,
		Timeout:        cfg.Plugins.Deps.Timeout,
		UseVenv:        cfg.Plugins.Deps.UseVenv,
		SharedVenvPath: cfg.Plugins.Deps.SharedVenvPath,
	})

	// ---- 插件系统 ----
	registry := plugin.NewRegistry()
	host := &runtime.Host{
		Bot:      b,
		Registry: registry,
		Perm:     perm,
		Redis:    redisSvc,
		MySQL:    mysqlSvc,
		Renderer: renderer,
		Deps:     depsMgr,
	}

	pluginDir := cfg.Plugins.Lua.Dir
	if pluginDir == "" {
		pluginDir = "plugins_lua"
	}
	absDir, _ := filepath.Abs(pluginDir)

	pyPluginDir := cfg.Plugins.Python.Dir
	if pyPluginDir == "" {
		pyPluginDir = "plugins_py"
	}
	absPyDir, _ := filepath.Abs(pyPluginDir)

	manager := plugin.NewManager(absDir, absPyDir, registry, host)
	if cfg.Plugins.Lua.Enabled || cfg.Plugins.Python.Enabled {
		if err := manager.LoadAll(ctx); err != nil {
			log.Warn("some plugins failed to load", "err", err.Error())
		}
	}
	log.Info("plugins loaded", "count", len(manager.Loaded()), "names", manager.Loaded())

	// 热重载
	watcher := plugin.NewWatcher(manager)
	go func() {
		if err := watcher.Run(ctx); err != nil && err != context.Canceled {
			log.Warn("watcher exited", "err", err.Error())
		}
	}()

	// 路由
	router := plugin.NewRouter(registry, b, perm, host)

	// 注入 ReverseWS 事件处理器 (延迟绑定, 因为 Router 创建在后)
	if revSrv != nil {
		revSrv.SetHandler(func(ctx context.Context, ev *event.Any) {
			router.Dispatch(ctx, ev)
		})
	}

	// 事件入口
	wsc.OnEvent(func(ctx context.Context, ev *event.Any) {
		if b.SelfID() == 0 {
			switch ev.Type {
			case event.PostMessage:
				b.SetSelfID(ev.Message.SelfID)
			case event.PostMeta:
				b.SetSelfID(ev.Meta.SelfID)
			}
		}
		router.Dispatch(ctx, ev)
	})

	log.Info("connecting to napcat", "uri", cfg.NapCatWS.URI)
	if err := wsc.Connect(ctx); err != nil && err != context.Canceled {
		log.Error("connect exited", "err", err.Error())
	}

	log.Info("closing")
	if manager != nil {
		_ = manager.Close()
	}
	if browserPool != nil {
		_ = browserPool.Close()
	}
	if redisSvc != nil {
		_ = redisSvc.(*redis.Service).Close()
	}
	if mysqlSvc != nil {
		_ = mysqlSvc.(*mysql.Service).Close()
	}
	_ = wsc.Close()
	log.Info("neobot-go exited cleanly")
}

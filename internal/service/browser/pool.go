// Package browser 基于 go-rod 的浏览器池.
package browser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// Config 浏览器配置.
type Config struct {
	Enabled      bool
	PoolSize     int
	ChromiumPath string // 空则使用 launcher 默认
	Headless     bool   // 默认 true
	Timeout      int    // 单次操作超时秒数, 默认 30
}

// Pool 浏览器实例池.
type Pool struct {
	cfg     Config
	logger  *slog.Logger

	browser *rod.Browser
	pages   chan *rod.Page
	mu      sync.Mutex
	closed  bool
}

// New 创建浏览器池. 立即启动 Chromium.
func New(cfg Config) (*Pool, error) {
	if !cfg.Enabled {
		return nil, nil // disabled, return nil 表示跳过
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 3
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}
	if cfg.Headless == false {
		cfg.Headless = true
	}

	p := &Pool{
		cfg:    cfg,
		logger: slog.Default().With("module", "browser.pool"),
		pages:  make(chan *rod.Page, cfg.PoolSize),
	}

	url, err := p.launchBrowser()
	if err != nil {
		return nil, err
	}
	p.logger.Info("browser launched", "url", url)

	if err := rod.Try(func() {
		p.browser = rod.New().ControlURL(url).MustConnect()
	}); err != nil {
		return nil, fmt.Errorf("connect browser: %w", err)
	}

	// 预热页面
	for i := 0; i < cfg.PoolSize; i++ {
		page := p.browser.MustPage()
		p.pages <- page
	}
	p.logger.Info("browser pool ready", "pool_size", cfg.PoolSize)
	return p, nil
}

func (p *Pool) launchBrowser() (string, error) {
	l := launcher.New().
		Headless(p.cfg.Headless).
		Set("no-sandbox").Set("disable-dev-shm-usage").
		Set("disable-gpu")
	if p.cfg.ChromiumPath != "" {
		l = l.Bin(p.cfg.ChromiumPath)
	}
	url, err := l.Launch()
	if err != nil {
		return "", fmt.Errorf("launch chromium: %w", err)
	}
	return url, nil
}

// AcquirePage 获取一个页面 (租借).
func (p *Pool) AcquirePage(ctx context.Context) (*rod.Page, error) {
	if p == nil || p.closed {
		return nil, errors.New("browser pool not initialized")
	}
	select {
	case page := <-p.pages:
		return page, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Duration(p.cfg.Timeout) * time.Second):
		return nil, errors.New("browser pool acquire timeout")
	}
}

// ReleasePage 归还页面.
func (p *Pool) ReleasePage(page *rod.Page) {
	if p == nil || p.closed {
		return
	}
	select {
	case p.pages <- page:
	default:
		// 池已满, 关闭该页面
		_ = page.Close()
	}
}

// Close 关闭浏览器.
func (p *Pool) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	close(p.pages)
	for page := range p.pages {
		_ = page.Close()
	}
	if p.browser != nil {
		if err := p.browser.Close(); err != nil {
			p.logger.Warn("browser close failed", "err", err.Error())
		}
	}
	p.logger.Info("browser pool closed")
	return nil
}
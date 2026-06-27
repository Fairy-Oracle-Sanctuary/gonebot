// Package image 提供 HTML 渲染为图片的能力 (基于 browser pool).
package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"neobot/core/internal/service/browser"
)

// Renderer 渲染器.
type Renderer struct {
	pool   *browser.Pool
	logger *slog.Logger
}

// New 创建渲染器.
func New(pool *browser.Pool) *Renderer {
	return &Renderer{
		pool:   pool,
		logger: slog.Default().With("module", "image.renderer"),
	}
}

// Options 渲染选项.
type Options struct {
	Width     int
	Height    int
	Quality   int
	FullPage  bool
	Format    string // png | jpeg
	Timeout   int
	UserAgent string
}

// DefaultOptions 默认渲染选项.
func DefaultOptions() Options {
	return Options{
		Width:    800,
		Height:   600,
		Quality:  90,
		FullPage: true,
		Format:   "png",
		Timeout:  30,
	}
}

// RenderHTML 适配 runtime.RendererService 接口 (width, quality 简化入口).
func (r *Renderer) RenderHTML(ctx context.Context, html string, width, quality int) ([]byte, error) {
	opts := DefaultOptions()
	if width > 0 {
		opts.Width = width
	}
	if quality > 0 {
		opts.Quality = quality
	}
	return r.renderHTMLOpts(ctx, html, opts)
}

// RenderURL 适配 runtime.RendererService 接口.
func (r *Renderer) RenderURL(ctx context.Context, url string, width, quality int) ([]byte, error) {
	opts := DefaultOptions()
	if width == 0 {
		width = 1280
	}
	opts.Width = width
	if quality > 0 {
		opts.Quality = quality
	}
	return r.renderURLOpts(ctx, url, opts)
}

// RenderTemplate 适配 runtime.RendererService 接口.
func (r *Renderer) RenderTemplate(ctx context.Context, tplStr string, data map[string]any, width, quality int) ([]byte, error) {
	t, err := template.New("inline").Parse(tplStr)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return r.RenderHTML(ctx, buf.String(), width, quality)
}

// renderHTMLOpts 内部实现.
func (r *Renderer) renderHTMLOpts(ctx context.Context, html string, opts Options) ([]byte, error) {
	if r.pool == nil {
		return nil, errors.New("renderer: browser pool not initialized")
	}
	if opts.Width == 0 {
		opts.Width = 800
	}
	if opts.Height == 0 {
		opts.Height = 600
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30
	}
	if opts.Format == "" {
		opts.Format = "png"
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
	defer cancel()

	page, err := r.pool.AcquirePage(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire page: %w", err)
	}
	defer r.pool.ReleasePage(page)

	if opts.UserAgent != "" {
		_ = page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: opts.UserAgent})
	}

	_ = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             opts.Width,
		Height:            opts.Height,
		DeviceScaleFactor: 1,
		Mobile:            false,
	})

	dataURL := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	if err := page.Navigate(dataURL); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		r.logger.Warn("wait load failed", "err", err.Error())
	}

	time.Sleep(300 * time.Millisecond)

	shotOpts := &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	}
	if strings.EqualFold(opts.Format, "jpeg") || strings.EqualFold(opts.Format, "jpg") {
		shotOpts.Format = proto.PageCaptureScreenshotFormatJpeg
		var q int = opts.Quality
		shotOpts.Quality = &q
	}

	img, err := page.Screenshot(opts.FullPage, shotOpts)
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	var buf bytes.Buffer
	buf.Write(img)
	return buf.Bytes(), nil
}

// renderURLOpts 内部实现.
func (r *Renderer) renderURLOpts(ctx context.Context, url string, opts Options) ([]byte, error) {
	if r.pool == nil {
		return nil, errors.New("renderer: browser pool not initialized")
	}
	if opts.Width == 0 {
		opts.Width = 1280
	}
	if opts.Height == 0 {
		opts.Height = 720
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
	defer cancel()

	page, err := r.pool.AcquirePage(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire page: %w", err)
	}
	defer r.pool.ReleasePage(page)

	_ = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             opts.Width,
		Height:            opts.Height,
		DeviceScaleFactor: 1,
		Mobile:            false,
	})

	if err := page.Navigate(url); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		r.logger.Warn("wait load failed", "err", err.Error())
	}

	time.Sleep(500 * time.Millisecond)

	img, err := page.Screenshot(opts.FullPage, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	if err != nil {
		return nil, fmt.Errorf("screenshot: %w", err)
	}
	var buf bytes.Buffer
	buf.Write(img)
	return buf.Bytes(), nil
}
package plugin

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher 监听插件目录变更并自动重载. 防抖 500ms.
type Watcher struct {
	manager *Manager
	logger  *slog.Logger
}

// NewWatcher 创建监听器.
func NewWatcher(m *Manager) *Watcher {
	return &Watcher{
		manager: m,
		logger:  slog.Default().With("module", "plugin.watcher"),
	}
}

// Run 阻塞直到 ctx 取消.
func (w *Watcher) Run(ctx context.Context) error {
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fs.Close()

	if err := fs.Add(w.manager.PluginDir()); err != nil {
		return err
	}
	if entries, err := readSubDirs(w.manager.PluginDir()); err == nil {
		for _, d := range entries {
			_ = fs.Add(d)
		}
	}

	w.logger.Info("watcher started", "dir", w.manager.PluginDir())

	debounce := make(map[string]*time.Timer)
	defer func() {
		for _, t := range debounce {
			t.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-fs.Events:
			if !ok {
				return nil
			}
			if !isPluginFile(ev.Name) {
				continue
			}
			name := extractPluginName(ev.Name, w.manager.PluginDir())
			if name == "" {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) == 0 {
				continue
			}
			if t, ok := debounce[name]; ok {
				t.Stop()
			}
			debounce[name] = time.AfterFunc(500*time.Millisecond, func() {
				w.logger.Info("file changed, reloading", "plugin", name, "file", ev.Name)
				if err := w.manager.Reload(ctx, name); err != nil {
					w.logger.Warn("reload failed", "plugin", name, "err", err.Error())
				}
			})
		case err, ok := <-fs.Errors:
			if !ok {
				return nil
			}
			w.logger.Warn("watcher error", "err", err.Error())
		}
	}
}

func isPluginFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".lua") || strings.HasSuffix(lower, ".toml")
}

func extractPluginName(filePath, root string) string {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return ""
	}
	name := parts[0]
	if name == "" || name[0] == '_' || name[0] == '.' {
		return ""
	}
	return name
}

func readSubDirs(root string) ([]string, error) {
	es, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(es))
	for _, e := range es {
		if e.IsDir() && e.Name()[0] != '_' && e.Name()[0] != '.' {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out, nil
}

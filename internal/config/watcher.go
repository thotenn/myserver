package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// Watcher watches config files for changes and triggers reload.
type Watcher struct {
	watcher  *fsnotify.Watcher
	logger   *zap.Logger
	onChange func()
	mu       sync.Mutex
	running  bool
}

// NewWatcher creates a new config file watcher.
func NewWatcher(logger *zap.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}
	return &Watcher{
		watcher: fsw,
		logger:  logger,
	}, nil
}

// Start begins watching the config directory for changes.
func (w *Watcher) Start(onChange func()) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return nil
	}

	w.onChange = onChange
	dir := ConfigDir()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("ensuring config dir: %w", err)
	}

	if err := w.watcher.Add(dir); err != nil {
		return fmt.Errorf("watching config dir %s: %w", dir, err)
	}

	go w.eventLoop()
	w.running = true
	w.logger.Info("config watcher started", zap.String("dir", dir))
	return nil
}

// Stop stops the watcher.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	w.running = false
	return w.watcher.Close()
}

func (w *Watcher) eventLoop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write | fsnotify.Create | fsnotify.Remove | fsnotify.Rename) {
				ext := filepath.Ext(event.Name)
				if ext == ".yaml" || ext == ".yml" || ext == ".css" || ext == ".js" {
					w.logger.Info("config file changed",
						zap.String("file", event.Name),
						zap.String("op", event.Op.String()),
					)
					if w.onChange != nil {
						w.onChange()
					}
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("config watcher error", zap.Error(err))
		}
	}
}

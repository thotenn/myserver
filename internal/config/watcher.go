package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// Watcher watches every dashboard's config directory for changes, plus the
// dashboards/ directory itself so a client added while the process is running
// starts being served without a restart.
//
// fsnotify is not recursive, so each directory is added explicitly and the
// set of watches is re-synced whenever the registry changes.
type Watcher struct {
	watcher  *fsnotify.Watcher
	logger   *zap.Logger
	onChange func(*Dashboard)
	mu       sync.Mutex
	running  bool
	watched  map[string]bool
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
		watched: map[string]bool{},
	}, nil
}

// Start begins watching the config tree. onChange is called with the
// dashboard whose configuration changed, AFTER it has been reloaded, so
// callers only have to invalidate what they own.
func (w *Watcher) Start(onChange func(*Dashboard)) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return nil
	}
	w.onChange = onChange

	root := Dashboards().Root()
	if err := os.MkdirAll(root.Dir, 0755); err != nil {
		return fmt.Errorf("ensuring config dir: %w", err)
	}
	if err := w.add(root.Dir); err != nil {
		return fmt.Errorf("watching config dir %s: %w", root.Dir, err)
	}
	w.syncLocked()

	go w.eventLoop()
	w.running = true
	w.logger.Info("config watcher started", zap.String("dir", root.Dir))
	return nil
}

// Sync brings the watch set in line with the current registry. It is called
// after every rescan and is safe to call at any time.
func (w *Watcher) Sync() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncLocked()
}

func (w *Watcher) syncLocked() {
	set := Dashboards()
	if set == nil {
		return
	}
	// The dashboards/ directory itself is watched even when it holds nothing
	// yet: its Create events are how a new client is noticed.
	dashboardsDir := filepath.Join(set.Root().Dir, DashboardsSubdir)
	if _, err := os.Stat(dashboardsDir); err == nil {
		if err := w.add(dashboardsDir); err != nil {
			w.logger.Warn("failed to watch the dashboards directory",
				zap.String("dir", dashboardsDir), zap.Error(err))
		}
	}
	for _, d := range set.Clients() {
		if w.watched[d.Dir] {
			continue
		}
		if err := w.add(d.Dir); err != nil {
			w.logger.Warn("failed to watch a dashboard directory",
				zap.String("dashboard", d.String()), zap.Error(err))
		}
	}
}

func (w *Watcher) add(dir string) error {
	if w.watched[dir] {
		return nil
	}
	if err := w.watcher.Add(dir); err != nil {
		return err
	}
	w.watched[dir] = true
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

// configExtensions are the file types a dashboard is built from.
func isConfigFile(name string) bool {
	switch filepath.Ext(name) {
	case ".yaml", ".yml", ".css", ".js":
		return true
	}
	return false
}

func (w *Watcher) eventLoop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write | fsnotify.Create | fsnotify.Remove | fsnotify.Rename) {
				continue
			}
			w.handle(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("config watcher error", zap.Error(err))
		}
	}
}

// handle routes one filesystem event: either a config file changed inside a
// dashboard, or a dashboard directory itself appeared or vanished.
func (w *Watcher) handle(event fsnotify.Event) {
	set := Dashboards()
	if set == nil {
		return
	}
	dir := filepath.Dir(event.Name)

	if dir == filepath.Join(set.Root().Dir, DashboardsSubdir) && !isConfigFile(event.Name) {
		w.rescan(event)
		return
	}

	if !isConfigFile(event.Name) {
		return
	}
	for _, d := range set.All() {
		if d.Dir != dir {
			continue
		}
		w.logger.Info("config file changed",
			zap.String("dashboard", d.String()),
			zap.String("file", filepath.Base(event.Name)),
			zap.String("op", event.Op.String()),
		)
		d.Reload()
		if w.onChange != nil {
			w.onChange(d)
		}
		return
	}
}

// rescan re-reads the dashboards directory after one of its entries appeared
// or disappeared, and starts serving whatever is now there.
//
// A directory is often created before the files inside it, so a dashboard that
// is new to this scan is reloaded explicitly: the writes that filled it may
// have landed before its watch existed.
func (w *Watcher) rescan(event fsnotify.Event) {
	before := Dashboards()
	set, errs := InitDashboards()
	for _, err := range errs {
		w.logger.Warn("ignoring a dashboard directory", zap.Error(err))
	}
	w.Sync()

	for _, d := range set.Clients() {
		if before != nil && before.Client(d.Slug) == d {
			continue
		}
		d.Reload()
		w.logger.Info("dashboard added", zap.String("dashboard", d.String()),
			zap.String("prefix", d.Prefix))
		if w.onChange != nil {
			w.onChange(d)
		}
	}
	if before != nil {
		for _, d := range before.Clients() {
			if set.Client(d.Slug) == nil {
				w.logger.Info("dashboard removed", zap.String("dashboard", d.String()))
			}
		}
	}
}

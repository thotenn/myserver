package scripts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager manages script registration, execution, and history.
type Manager struct {
	scripts        map[string]*ScriptConfig
	running        map[string]*Execution
	history        map[string][]*Execution
	auditLog       *AuditLogger
	executor       *Executor
	scriptDirs     []string
	defaultTimeout int
	maxTimeout     int
	sem            chan struct{}
	mu             sync.RWMutex
}

// NewManager creates a new script manager.
//   - scriptDirs: whitelisted directories containing .sh scripts.
//   - defaultTimeout: seconds used when a script doesn't declare its own.
//   - maxTimeout: hard cap in seconds (0 = no cap).
//   - maxConcurrent: global semaphore size for the total number of scripts
//     that can run simultaneously (must be > 0).
func NewManager(scriptDirs []string, defaultTimeout, maxTimeout, maxConcurrent int) *Manager {
	if defaultTimeout <= 0 {
		defaultTimeout = 60
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	return &Manager{
		scripts:        make(map[string]*ScriptConfig),
		running:        make(map[string]*Execution),
		history:        make(map[string][]*Execution),
		auditLog:       NewAuditLogger(),
		executor:       NewExecutor(),
		scriptDirs:     scriptDirs,
		defaultTimeout: defaultTimeout,
		maxTimeout:     maxTimeout,
		sem:            make(chan struct{}, maxConcurrent),
	}
}

// Register validates and registers a script config.
// Returns an error if the script fails validation.
func (m *Manager) Register(cfg *ScriptConfig) error {
	if err := m.validateScript(cfg); err != nil {
		return fmt.Errorf("script %q: %w", cfg.Name, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scripts[cfg.Name] = cfg
	return nil
}

// ReplaceAll atomically replaces the script registry. Running executions are
// preserved. Called by the config watcher when scripts.yaml changes.
func (m *Manager) ReplaceAll(newScripts []*ScriptConfig) []error {
	validated := make(map[string]*ScriptConfig, len(newScripts))
	var errs []error
	for _, cfg := range newScripts {
		if err := m.validateScript(cfg); err != nil {
			errs = append(errs, fmt.Errorf("script %q: %w", cfg.Name, err))
			continue
		}
		validated[cfg.Name] = cfg
	}
	m.mu.Lock()
	m.scripts = validated
	m.mu.Unlock()
	return errs
}

// Has checks if a script is registered by name.
func (m *Manager) Has(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.scripts[name]
	return ok
}

// Get returns a registered script config.
func (m *Manager) Get(name string) (*ScriptConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg, ok := m.scripts[name]
	return cfg, ok
}

// List returns all registered script statuses (copy, safe to mutate).
func (m *Manager) List() []ScriptStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]ScriptStatus, 0, len(m.scripts))
	for name, cfg := range m.scripts {
		status := ScriptStatus{
			Name:           name,
			Description:    cfg.Description,
			Icon:           cfg.Icon,
			Running:        false,
			RequireConfirm: cfg.RequireConfirm,
		}
		if _, ok := m.running[name]; ok {
			status.Running = true
		}
		if hist := m.history[name]; len(hist) > 0 {
			last := hist[len(hist)-1]
			status.LastStatus = last.Status
			status.LastExitCode = last.ExitCode
		}
		statuses = append(statuses, status)
	}
	return statuses
}

// resolveTimeout returns the timeout (in seconds) for a given script config,
// honouring default and max caps.
func (m *Manager) resolveTimeout(cfg *ScriptConfig) int {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = m.defaultTimeout
	}
	if m.maxTimeout > 0 && timeout > m.maxTimeout {
		timeout = m.maxTimeout
	}
	if timeout <= 0 {
		timeout = 60
	}
	return timeout
}

// Run executes a registered script by name.
// The supplied context is used as the parent for the execution timeout, so
// cancelling it (e.g. when the HTTP client disconnects) will kill the script.
func (m *Manager) Run(ctx context.Context, name, clientIP string) (*Execution, error) {
	m.mu.Lock()
	cfg, ok := m.scripts[name]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("script %q not found", name)
	}

	// Concurrency check (same-name).
	if _, running := m.running[name]; running && !cfg.AllowConcurrent {
		m.mu.Unlock()
		return nil, errors.New("script is already running")
	}

	ex := &Execution{
		Name:      name,
		Status:    "running",
		StartedAt: time.Now(),
		ClientIP:  clientIP,
	}
	m.running[name] = ex
	m.mu.Unlock()

	// Global concurrency: block until a slot is free or the client cancels.
	select {
	case m.sem <- struct{}{}:
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.running, name)
		m.mu.Unlock()
		return nil, ctx.Err()
	}
	defer func() { <-m.sem }()

	timeout := m.resolveTimeout(cfg)
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	result := m.executor.Execute(execCtx, cfg, clientIP)
	result.ClientIP = clientIP

	m.mu.Lock()
	delete(m.running, name)
	m.history[name] = append(m.history[name], result)
	if len(m.history[name]) > 10 {
		m.history[name] = m.history[name][len(m.history[name])-10:]
	}
	m.mu.Unlock()

	m.auditLog.Log(result)
	return result, nil
}

// Stream runs a script and returns a channel of output lines. The returned
// channel is closed when the script finishes (success, error, or timeout).
// Cancelling ctx kills the script and its children.
func (m *Manager) Stream(ctx context.Context, name, clientIP string) (<-chan string, error) {
	m.mu.Lock()
	cfg, ok := m.scripts[name]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("script %q not found", name)
	}
	if _, running := m.running[name]; running && !cfg.AllowConcurrent {
		m.mu.Unlock()
		return nil, errors.New("script is already running")
	}

	ex := &Execution{
		Name:      name,
		Status:    "running",
		StartedAt: time.Now(),
		ClientIP:  clientIP,
	}
	m.running[name] = ex
	m.mu.Unlock()

	// Acquire concurrency slot before spawning the goroutine so a flood of
	// calls cannot outrun the semaphore.
	select {
	case m.sem <- struct{}{}:
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.running, name)
		m.mu.Unlock()
		return nil, ctx.Err()
	}

	timeout := m.resolveTimeout(cfg)
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	lines := make(chan string, 100)

	go func() {
		defer close(lines)
		defer cancel()
		defer func() { <-m.sem }()

		result := m.executor.StreamOutput(execCtx, cfg, lines)
		result.ClientIP = clientIP

		m.mu.Lock()
		delete(m.running, name)
		m.history[name] = append(m.history[name], result)
		if len(m.history[name]) > 10 {
			m.history[name] = m.history[name][len(m.history[name])-10:]
		}
		m.mu.Unlock()

		m.auditLog.Log(result)
	}()

	return lines, nil
}

// Status returns the current status of a script.
func (m *Manager) Status(name string) (*ScriptStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg, ok := m.scripts[name]
	if !ok {
		return nil, fmt.Errorf("script %q not found", name)
	}

	status := &ScriptStatus{
		Name:           name,
		Description:    cfg.Description,
		Icon:           cfg.Icon,
		Running:        false,
		RequireConfirm: cfg.RequireConfirm,
	}

	if _, running := m.running[name]; running {
		status.Running = true
	}
	if hist := m.history[name]; len(hist) > 0 {
		last := hist[len(hist)-1]
		status.LastStatus = last.Status
		status.LastExitCode = last.ExitCode
	}

	return status, nil
}

// dangerousEnvVars are variables whose values affect the bash process itself
// or dynamic linker loading. They must not come from scripts.yaml.
var dangerousEnvVars = map[string]struct{}{
	"LD_PRELOAD":      {},
	"LD_LIBRARY_PATH": {},
	"LD_AUDIT":        {},
	"BASH_ENV":        {},
	"ENV":             {},
	"PROMPT_COMMAND":  {},
	"IFS":             {},
	"PATH":            {},
}

// validateScript validates that a script config points to a valid .sh file
// within one of the whitelisted directories and that its env vars do not
// contain dangerous names.
func (m *Manager) validateScript(cfg *ScriptConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("name is required")
	}
	if cfg.Command == "" {
		return fmt.Errorf("command is required")
	}

	// Reject absolute paths outright — command must be relative to one of
	// the whitelisted scriptDirs.
	if filepath.IsAbs(cfg.Command) {
		return fmt.Errorf("command must be relative to a whitelisted dir, got absolute: %s", cfg.Command)
	}

	// Deny dangerous env vars before even trying to resolve the path.
	for k := range cfg.Env {
		if _, bad := dangerousEnvVars[k]; bad {
			return fmt.Errorf("env var %q is not allowed", k)
		}
		if strings.HasPrefix(k, "BASH_FUNC_") {
			return fmt.Errorf("env var %q is not allowed", k)
		}
	}

	var lastErr error
	for _, dir := range m.scriptDirs {
		cleanDir := filepath.Clean(dir)
		// BOTH sides of the containment check have to be symlink-resolved.
		// Comparing a resolved path against an unresolved directory reports a
		// traversal whenever the directory itself sits behind a symlink —
		// which is every temp dir on macOS (/var -> /private/var), so the
		// whole test suite failed there while production on Linux was fine.
		// Resolved-vs-resolved is also the stricter comparison: it is what
		// actually answers "is the file I am about to run inside the
		// directory I trust".
		realDir, err := filepath.EvalSymlinks(cleanDir)
		if err != nil {
			if !os.IsNotExist(err) {
				lastErr = err
			}
			continue
		}
		candidate := filepath.Join(realDir, cfg.Command)
		real, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			lastErr = err
			continue
		}

		// Enforce that the resolved path is strictly inside realDir (with
		// separator) to defeat prefix-collision attacks like /app/scriptsbak/.
		if real != realDir && !strings.HasPrefix(real, realDir+string(os.PathSeparator)) {
			return fmt.Errorf("path traversal detected: %s resolves outside %s", cfg.Command, cleanDir)
		}

		// Only .sh files (case-sensitive, matches Linux filesystem semantics).
		if filepath.Ext(real) != ".sh" {
			return fmt.Errorf("only .sh scripts allowed, got: %s", filepath.Ext(real))
		}

		// Sanity check on file mode. We execute via /bin/bash so the exec bit
		// is not strictly required, but refusing non-regular files is
		// important (e.g. devices, symlinks that resolved to a device).
		info, err := os.Stat(real)
		if err != nil {
			return fmt.Errorf("stat failed: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("script is not a regular file: %s", real)
		}
		if info.Mode().Perm()&0o002 != 0 {
			return fmt.Errorf("script is world-writable: %s", real)
		}

		// Also verify the containing directory is not world-writable.
		dirInfo, err := os.Stat(realDir)
		if err == nil && dirInfo.Mode().Perm()&0o002 != 0 {
			return fmt.Errorf("script directory is world-writable: %s", cleanDir)
		}

		cfg.resolvedPath = real
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("script %s not reachable in any whitelisted dir: %w", cfg.Command, lastErr)
	}
	return fmt.Errorf("script %s not found in any whitelisted directory", cfg.Command)
}

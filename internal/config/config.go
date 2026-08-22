package config

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

//go:embed skeleton
var skeletonFS embed.FS

var (
	configDir string
	mu        sync.RWMutex
	resolved  bool

	// liveHash holds the current config hash, updated by the file watcher.
	// Use CurrentHash() / SetCurrentHash() instead of reading a local copy
	// that may freeze after hot-reload.
	liveHash atomic.Value // string
)

// CurrentHash returns the most recently computed config hash.
// It is thread-safe and updated whenever the watcher detects a change.
func CurrentHash() string {
	v, _ := liveHash.Load().(string)
	return v
}

// SetCurrentHash stores the current config hash (called by the watcher).
func SetCurrentHash(h string) {
	liveHash.Store(h)
}

// ConfigDir returns the configuration directory path.
// It reads from HOMEPAGE_CONFIG_DIR env var, defaulting to /app/config.
func ConfigDir() string {
	mu.RLock()
	if resolved {
		mu.RUnlock()
		return configDir
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()
	// Double-check after acquiring write lock
	if resolved {
		return configDir
	}
	configDir = os.Getenv("HOMEPAGE_CONFIG_DIR")
	if configDir == "" {
		configDir = "/app/config"
	}
	resolved = true
	return configDir
}

// SetConfigDir overrides the config directory (for testing).
func SetConfigDir(dir string) {
	mu.Lock()
	defer mu.Unlock()
	configDir = dir
	resolved = true
}

// ResetConfigDir resets the config dir resolution (for testing).
func ResetConfigDir() {
	mu.Lock()
	defer mu.Unlock()
	configDir = ""
	resolved = false
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config dir %s: %w", dir, err)
	}
	return nil
}

// CheckAndCopyConfig ensures a config file exists by copying from skeleton.
// If the file exists, it does nothing. If not, it copies the skeleton version.
func CheckAndCopyConfig(filename string) error {
	dir := ConfigDir()
	destPath := filepath.Join(dir, filename)

	if _, err := os.Stat(destPath); err == nil {
		return nil // file already exists
	}

	skeletonData, err := fs.ReadFile(skeletonFS, filepath.Join("skeleton", filename))
	if err != nil {
		// No skeleton for this file, that's ok
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating parent dir for %s: %w", destPath, err)
	}

	if err := os.WriteFile(destPath, skeletonData, 0644); err != nil {
		return fmt.Errorf("writing skeleton %s: %w", destPath, err)
	}

	return nil
}

// ReadConfigFile reads a config file with environment variable substitution.
func ReadConfigFile(filename string) ([]byte, error) {
	path := filepath.Join(ConfigDir(), filename)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", filename, err)
	}
	substituted := SubstituteEnvVars(string(raw))
	return []byte(substituted), nil
}

// ConfigHash computes a SHA256 hash of all config files for cache busting.
// Includes YAMLs and custom.css/custom.js so changes to any of them force a
// frontend reload via the /api/hash endpoint.

// ConfigHash returns the cached config if available, otherwise loads from disk.
func ConfigHash() (string, error) {
	if c := GetCachedConfig(); c != nil {

		return c.Hash, nil
	}
	return configHash()
}
func configHash() (string, error) {
	h := sha256.New()
	dir := ConfigDir()

	configFiles := []string{
		"services.yaml", "bookmarks.yaml", "widgets.yaml",
		"docker.yaml", "kubernetes.yaml", "proxmox.yaml",
		"settings.yaml", "scripts.yaml", AuthFile,
		"custom.css", "custom.js",
	}

	for _, f := range configFiles {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("hashing config %s: %w", f, err)
		}
		h.Write([]byte(f))
		h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

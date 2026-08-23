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
)

//go:embed skeleton
var skeletonFS embed.FS

// skeletonFiles are the config files a brand-new install is seeded with. They
// are copied once, at startup, into the ROOT config directory only: a client
// dashboard's directory is written by the operator, and seeding it would drop
// the demo dashboard into somebody else's.
var skeletonFiles = []string{
	"services.yaml", "bookmarks.yaml", "widgets.yaml", "settings.yaml",
	"docker.yaml", "kubernetes.yaml", "proxmox.yaml", "scripts.yaml",
}

var (
	configDir string
	mu        sync.RWMutex
	resolved  bool
)

// ConfigDir returns the ROOT configuration directory path. It reads from
// HOMEPAGE_CONFIG_DIR, defaulting to /app/config.
//
// It is not "the config dir a handler should read": that is
// DashboardFrom(ctx).Dir. This one only tells the registry and the watcher
// where the tree starts.
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

// SetConfigDir overrides the root config directory (for testing).
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

// EnsureConfigDir creates the root configuration directory if it doesn't exist.
func EnsureConfigDir() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config dir %s: %w", dir, err)
	}
	return nil
}

// EnsureSkeleton seeds a config directory with the shipped example files,
// skipping any that already exist.
//
// It runs once at startup rather than from inside the loaders, which is what
// makes it possible for a directory to legitimately hold only some of the
// config files: a client dashboard with a services.yaml and nothing else is a
// complete dashboard, not a half-seeded one.
func EnsureSkeleton(dir string) error {
	for _, name := range skeletonFiles {
		destPath := filepath.Join(dir, name)
		if _, err := os.Stat(destPath); err == nil {
			continue // file already exists
		}

		data, err := fs.ReadFile(skeletonFS, filepath.Join("skeleton", name))
		if err != nil {
			continue // no skeleton for this file, that's ok
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating parent dir for %s: %w", destPath, err)
		}
		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("writing skeleton %s: %w", destPath, err)
		}
	}
	return nil
}

// readConfigFile reads a config file from a dashboard's directory with
// environment variable substitution.
//
// A missing file is not an error: it returns nil data, and every loader reads
// that as "nothing configured". That is what lets a dashboard directory hold
// only the files its operator wrote.
func readConfigFile(dir, filename string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config %s: %w", filename, err)
	}
	return []byte(SubstituteEnvVars(string(raw))), nil
}

// configHash computes a SHA256 hash of a dashboard's config files for cache
// busting. It covers the YAMLs plus custom.css/custom.js, so a change to any
// of them forces that dashboard's frontend to reload via /api/hash — and only
// that dashboard's.
func configHash(dir string) (string, error) {
	h := sha256.New()

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

package config

import "sync/atomic"

// cachedConfig holds all parsed configuration in memory.
// It is updated atomically by the watcher so handlers always see a
// consistent snapshot without hitting disk on every request.
type cachedConfig struct {
	Services   []ServiceGroup
	Bookmarks  []BookmarkGroup
	Widgets    []InfoWidget
	Settings   *Settings
	Docker     map[string]DockerConfig
	Kubernetes map[string]KubernetesConfig
	Proxmox    map[string]ProxmoxConfig
	Scripts    *ScriptsFile
	Hash       string
}

var configCache atomic.Value // *cachedConfig

// GetCachedConfig returns the current in-memory config, or nil if the cache
// has not been initialised yet.
func GetCachedConfig() *cachedConfig {
	if c, ok := configCache.Load().(*cachedConfig); ok {
		return c
	}
	return nil
}

// ReloadCache loads all configuration files from disk and atomically swaps
// the in-memory cache. It should be called at startup and whenever the file
// watcher detects a change.
func ReloadCache() {
	c := &cachedConfig{}
	c.Services, _ = loadServices()
	c.Bookmarks, _ = loadBookmarks()
	c.Widgets, _ = loadWidgets()
	c.Settings, _ = loadSettings()
	c.Docker, _ = loadDocker()
	c.Kubernetes, _ = loadKubernetes()
	c.Proxmox, _ = loadProxmox()
	c.Scripts, _ = loadScriptsFile()
	c.Hash, _ = configHash()
	configCache.Store(c)
	SetCurrentHash(c.Hash)
}

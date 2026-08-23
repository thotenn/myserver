package config

// cachedConfig holds one dashboard's parsed configuration in memory.
// A Dashboard swaps it atomically so handlers always see a consistent
// snapshot of THEIR dashboard without hitting disk on every request.
//
// There is deliberately no process-wide instance of this. There used to be,
// and it is what made /api/services able to answer one dashboard's request
// with another's services.
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

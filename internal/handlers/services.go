package handlers

import (
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/discovery"
)

var (
	dockerDiscoverers   []*discovery.DockerDiscoverer
	mergedServicesCache atomic.Value // []config.ServiceGroup
	lastMergedHash      atomic.Value // string
)

// SetDockerDiscoverers sets the Docker discoverers used by the services handler.
func SetDockerDiscoverers(discoverers []*discovery.DockerDiscoverer) {
	dockerDiscoverers = discoverers
}

// Services handles GET /api/services. It returns the merged service list
// after stripping credentials and basic-auth URLs from each entry.
func Services(w http.ResponseWriter, r *http.Request) {
	currentHash := config.CurrentHash()

	// Return cached result if hash hasn't changed.
	if cached, ok := mergedServicesCache.Load().([]config.ServiceGroup); ok {
		if lastHash, _ := lastMergedHash.Load().(string); lastHash == currentHash && currentHash != "" {
			writeServices(w, cached)
			return
		}
	}

	services, err := config.LoadServices()
	if err != nil {
		http.Error(w, "failed to load services", http.StatusInternalServerError)
		return
	}

	// Discover Docker services
	var discovered []config.ServiceGroup
	for _, d := range dockerDiscoverers {
		groups, err := d.DiscoverServices(r.Context())
		if err != nil {
			continue
		}
		discovered = append(discovered, groups...)
	}

	settings, _ := config.LoadSettings()
	var layout map[string]config.LayoutGroup
	if settings != nil {
		layout = settings.Layout
	}
	merged := discovery.MergeServices(services, discovered, layout)

	mergedServicesCache.Store(merged)
	lastMergedHash.Store(currentHash)

	writeServices(w, merged)
}

func writeServices(w http.ResponseWriter, services []config.ServiceGroup) {
	for i := range services {
		for j := range services[i].Services {
			services[i].Services[j] = config.SanitizeService(services[i].Services[j])
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(services)
}

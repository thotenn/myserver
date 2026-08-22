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

	// Return cached result if hash hasn't changed. What is cached is already
	// sanitized, so it is written out as-is — never post-processed in place.
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
	sanitized := sanitizeServiceGroups(merged)

	mergedServicesCache.Store(sanitized)
	lastMergedHash.Store(currentHash)

	writeServices(w, sanitized)
}

// sanitizeServiceGroups returns a sanitized COPY of groups. The group slice
// and each group's service slice are freshly allocated, and
// config.SanitizeService rebuilds the widget rather than editing it, so the
// result shares no mutable state with the caller's input.
//
// Copying is not cosmetic. The previous version sanitized in place, which
// meant every request wrote to the same slice held in mergedServicesCache
// while other requests were serializing it — a data race on a large struct,
// and a torn read is a plausible outcome. Any future derivation of a
// per-request view (for example filtering the list by the caller's
// identity) must follow the same rule: mutating the cached slice would let
// one request's view leak into every other request's response.
//
// nil in, nil out: MergeServices returns nil when there is nothing to show
// and the endpoint has always encoded that as JSON `null`.
func sanitizeServiceGroups(groups []config.ServiceGroup) []config.ServiceGroup {
	if groups == nil {
		return nil
	}
	out := make([]config.ServiceGroup, len(groups))
	for i, g := range groups {
		var svcs []config.Service
		if g.Services != nil {
			svcs = make([]config.Service, len(g.Services))
			for j, s := range g.Services {
				svcs[j] = config.SanitizeService(s)
			}
		}
		out[i] = config.ServiceGroup{Name: g.Name, Services: svcs}
	}
	return out
}

// writeServices encodes an already-sanitized group list. It must not modify
// the value it is given: the same slice is shared with mergedServicesCache.
func writeServices(w http.ResponseWriter, services []config.ServiceGroup) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(services)
}

package handlers

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/discovery"
)

// dockerDiscoverers are built from the ROOT dashboard's docker.yaml and talk
// to the host's daemon. They are never merged into a client dashboard's
// services: a container running on my host is not part of a client's list.
var dockerDiscoverers []*discovery.DockerDiscoverer

// merged is one dashboard's already-sanitized service list, cached against the
// config hash it was built from.
type merged struct {
	hash   string
	groups []config.ServiceGroup
}

// mergedServicesCache is keyed by dashboard slug, and that key is the whole
// point. It used to be a single process-wide value guarded by a single
// process-wide hash, which meant GET /acme/api/services answered with whatever
// the previous request — for any dashboard — had left in it. A data leak
// between clients with no error to notice.
var mergedServicesCache sync.Map // slug -> merged

// SetDockerDiscoverers sets the Docker discoverers used by the services handler.
func SetDockerDiscoverers(discoverers []*discovery.DockerDiscoverer) {
	dockerDiscoverers = discoverers
}

// Services handles GET /api/services. It returns the merged service list
// after stripping credentials and basic-auth URLs from each entry.
func Services(w http.ResponseWriter, r *http.Request) {
	d, ok := dashboardOf(w, r)
	if !ok {
		return
	}
	currentHash := d.Hash()

	// Return cached result if this dashboard's hash hasn't changed. What is
	// cached is already sanitized, so it is written out as-is — never
	// post-processed in place.
	if entry, found := mergedServicesCache.Load(d.Slug); found {
		if m, isMerged := entry.(merged); isMerged && m.hash == currentHash && currentHash != "" {
			writeServices(w, m.groups)
			return
		}
	}

	services, err := d.Services()
	if err != nil {
		http.Error(w, "failed to load services", http.StatusInternalServerError)
		return
	}

	// Discover Docker services. Only for the root dashboard: the containers
	// found here run on the host, and folding them into a client's list would
	// publish the host's inventory to that client.
	var discovered []config.ServiceGroup
	if d.IsRoot() {
		for _, disc := range dockerDiscoverers {
			groups, err := disc.DiscoverServices(r.Context())
			if err != nil {
				continue
			}
			discovered = append(discovered, groups...)
		}
	}

	settings, _ := d.Settings()
	var layout map[string]config.LayoutGroup
	if settings != nil {
		layout = settings.Layout
	}
	sanitized := sanitizeServiceGroups(discovery.MergeServices(services, discovered, layout))

	mergedServicesCache.Store(d.Slug, merged{hash: currentHash, groups: sanitized})

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

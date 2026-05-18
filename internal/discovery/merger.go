package discovery

import (
	"sort"

	"github.com/thotenn/myserver/internal/config"
)

// MergeServices merges services from config, Docker, and Kubernetes sources.
// Config services take priority (by name), then Docker, then K8s.
// Groups are sorted by layout order if defined, then alphabetically.
func MergeServices(
	configGroups []config.ServiceGroup,
	dockerGroups []config.ServiceGroup,
	layout map[string]config.LayoutGroup,
) []config.ServiceGroup {
	merged := make(map[string][]config.Service)

	// Add Docker-discovered services first (lower priority)
	for _, g := range dockerGroups {
		for _, s := range g.Services {
			merged[g.Name] = append(merged[g.Name], s)
		}
	}

	// Add config services (higher priority, overrides Docker by name)
	for _, g := range configGroups {
		for _, s := range g.Services {
			// Check if service already exists from Docker
			existing := findServiceByName(merged[g.Name], s.Name)
			if existing != nil {
				// Merge: config overrides Docker fields
				mergeService(existing, s)
			} else {
				merged[g.Name] = append(merged[g.Name], s)
			}
		}
	}

	// Sort services within groups by weight, then name
	for name := range merged {
		sortServices(merged[name])
	}

	// Sort groups by layout order, then alphabetically for unlisted
	var groups []config.ServiceGroup
	groups = sortByLayout(merged, layout)

	return groups
}

// findServiceByName finds a service by name in a slice.
func findServiceByName(services []config.Service, name string) *config.Service {
	for i := range services {
		if services[i].Name == name {
			return &services[i]
		}
	}
	return nil
}

// mergeService merges override fields from src into dst.
func mergeService(dst *config.Service, src config.Service) {
	if src.Href != "" {
		dst.Href = src.Href
	}
	if src.Icon != "" {
		dst.Icon = src.Icon
	}
	if src.Description != "" {
		dst.Description = src.Description
	}
	if src.Widget != nil {
		dst.Widget = src.Widget
	}
	if src.Ping != "" {
		dst.Ping = src.Ping
	}
	if src.SiteMonitor != "" {
		dst.SiteMonitor = src.SiteMonitor
	}
	if src.Weight != 0 {
		dst.Weight = src.Weight
	}
	if src.Type != "" {
		dst.Type = src.Type
	}
	if src.Script != "" {
		dst.Script = src.Script
	}
}

// sortServices sorts services by weight (ascending), then by name.
func sortServices(services []config.Service) {
	sort.SliceStable(services, func(i, j int) bool {
		wi, wj := services[i].Weight, services[j].Weight
		if wi != wj {
			return wi < wj
		}
		return services[i].Name < services[j].Name
	})
}

// sortByLayout sorts groups according to layout configuration.
//
// Because Go's map iteration is non-deterministic, group ordering for the
// layout map is alphabetical. The Node.js Homepage settings.yaml supports a
// list-of-maps form that preserves order; that fix is tracked separately
// (see review/CHECKLIST.md, settings.yaml legacy layout finding). Until
// then, alphabetical ordering is at least deterministic.
func sortByLayout(merged map[string][]config.Service, layout map[string]config.LayoutGroup) []config.ServiceGroup {
	inLayout := make(map[string]bool, len(layout))
	for name := range layout {
		inLayout[name] = true
	}

	var layoutNames []string
	for name := range layout {
		layoutNames = append(layoutNames, name)
	}
	sort.Strings(layoutNames)

	var groups []config.ServiceGroup
	for _, name := range layoutNames {
		if services, ok := merged[name]; ok {
			groups = append(groups, config.ServiceGroup{
				Name:     name,
				Services: services,
			})
		}
	}

	var remaining []string
	for name := range merged {
		if !inLayout[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	for _, name := range remaining {
		groups = append(groups, config.ServiceGroup{
			Name:     name,
			Services: merged[name],
		})
	}

	return groups
}

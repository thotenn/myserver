package templates

import (
	"context"
	"fmt"
	"net/url"

	"github.com/thotenn/myserver/internal/config"
)

// Every URL this package emits is absolute from the host root, so each one has
// to carry the base path of the dashboard being rendered. The prefix travels in
// the context (see internal/config/basepath.go) rather than through PageData:
// inside a templ component `ctx` is always in scope, which is what lets a card
// deep in the tree build a correct URL without ten component signatures
// growing a parameter.
//
// With no base path every builder returns exactly the string it returned
// before the feature existed.

// prefixed prefixes a dashboard-relative absolute path with the base path.
func prefixed(ctx context.Context, p string) string {
	return config.PrefixPathFrom(ctx, p)
}

// staticURL builds a /static/* URL with the asset cache-busting version.
// The version is handlers.AssetVersion() — a hash of the BUILD OUTPUT, never
// of the config.
func staticURL(ctx context.Context, p, version string) string {
	u := prefixed(ctx, p)
	if version == "" {
		return u
	}
	return u + "?v=" + version
}

// configFileURL builds a /api/config/<file> URL for operator-supplied assets
// (custom.css, custom.js).
func configFileURL(ctx context.Context, file string) string {
	return prefixed(ctx, "/api/config/"+file)
}

// authURL builds an /auth/* URL.
func authURL(ctx context.Context, p string) string {
	return prefixed(ctx, p)
}

// pingURL builds the /api/ping URL with properly-escaped query params so
// service names with special characters (&, =, spaces) do not corrupt the
// query string.
func pingURL(ctx context.Context, group, service string) string {
	return prefixed(ctx, "/api/ping") + "?groupName=" + url.QueryEscape(group) +
		"&serviceName=" + url.QueryEscape(service)
}

// siteMonitorURL builds the /api/siteMonitor URL with escaped params.
func siteMonitorURL(ctx context.Context, group, service string) string {
	return prefixed(ctx, "/api/siteMonitor") + "?groupName=" + url.QueryEscape(group) +
		"&serviceName=" + url.QueryEscape(service)
}

// proxyURL builds a /api/services/proxy URL with escaped params.
func proxyURL(ctx context.Context, group, service, endpoint string) string {
	if endpoint == "" {
		endpoint = "default"
	}
	return prefixed(ctx, "/api/services/proxy") + "?group=" + url.QueryEscape(group) +
		"&service=" + url.QueryEscape(service) +
		"&endpoint=" + url.QueryEscape(endpoint)
}

// dockerStatsURL builds a /api/docker/stats URL with escaped path segments.
func dockerStatsURL(ctx context.Context, container, server string) string {
	return prefixed(ctx, "/api/docker/stats/") + url.PathEscape(container) + "/" + url.PathEscape(server)
}

// dockerStatusURL builds a /api/docker/status URL with escaped path segments.
func dockerStatusURL(ctx context.Context, container, server string) string {
	return prefixed(ctx, "/api/docker/status/") + url.PathEscape(container) + "/" + url.PathEscape(server)
}

// scriptRunURL builds the /api/scripts/<name> URL the execute button posts to.
// Script names are validated at registration, but the escape stays: building an
// API URL by bare concatenation is how a name with a slash or a `?` in it turns
// into a different request than the one intended.
func scriptRunURL(ctx context.Context, name string) string {
	return prefixed(ctx, "/api/scripts/") + url.PathEscape(name)
}

// resourcesURL builds the /api/widgets/resources URL with the configured
// widget options serialised as query parameters so the handler knows which
// metric to compute (cpu, memory, disk, uptime, etc).
func resourcesURL(ctx context.Context, opts map[string]interface{}) string {
	q := url.Values{}
	// Only propagate known keys as strings.
	for _, k := range []string{"label", "cpu", "cputemp", "memory", "disk", "uptime", "expanded", "network"} {
		if v, ok := opts[k]; ok && v != nil {
			q.Set(k, fmt.Sprintf("%v", v))
		}
	}
	return prefixed(ctx, "/api/widgets/resources") + "?" + q.Encode()
}

// weatherURL builds the /api/widgets/openmeteo URL.
func weatherURL(ctx context.Context, opts map[string]interface{}) string {
	q := url.Values{}
	for _, k := range []string{"label", "latitude", "longitude", "timezone", "units", "cache"} {
		if v, ok := opts[k]; ok && v != nil {
			q.Set(k, fmt.Sprintf("%v", v))
		}
	}
	return prefixed(ctx, "/api/widgets/openmeteo") + "?" + q.Encode()
}

// basePathOf exposes the prefix to the templates that have to hand it to the
// browser — the `base-path` meta tag app.js reads. It stays a function rather
// than a PageData field so there is one source of truth per request.
func basePathOf(ctx context.Context) string {
	return config.BasePathFrom(ctx)
}

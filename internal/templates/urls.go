package templates

import (
	"fmt"
	"net/url"
)

// pingURL builds the /api/ping URL with properly-escaped query params so
// service names with special characters (&, =, spaces) do not corrupt the
// query string.
func pingURL(group, service string) string {
	return "/api/ping?groupName=" + url.QueryEscape(group) +
		"&serviceName=" + url.QueryEscape(service)
}

// siteMonitorURL builds the /api/siteMonitor URL with escaped params.
func siteMonitorURL(group, service string) string {
	return "/api/siteMonitor?groupName=" + url.QueryEscape(group) +
		"&serviceName=" + url.QueryEscape(service)
}

// proxyURL builds a /api/services/proxy URL with escaped params.
func proxyURL(group, service, endpoint string) string {
	if endpoint == "" {
		endpoint = "default"
	}
	return "/api/services/proxy?group=" + url.QueryEscape(group) +
		"&service=" + url.QueryEscape(service) +
		"&endpoint=" + url.QueryEscape(endpoint)
}

// dockerStatsURL builds a /api/docker/stats URL with escaped path segments.
func dockerStatsURL(container, server string) string {
	return "/api/docker/stats/" + url.PathEscape(container) + "/" + url.PathEscape(server)
}

// dockerStatusURL builds a /api/docker/status URL with escaped path segments.
func dockerStatusURL(container, server string) string {
	return "/api/docker/status/" + url.PathEscape(container) + "/" + url.PathEscape(server)
}

// resourcesURL builds the /api/widgets/resources URL with the configured
// widget options serialised as query parameters so the handler knows which
// metric to compute (cpu, memory, disk, uptime, etc).
func resourcesURL(opts map[string]interface{}) string {
	q := url.Values{}
	// Only propagate known keys as strings.
	for _, k := range []string{"label", "cpu", "cputemp", "memory", "disk", "uptime", "expanded", "network"} {
		if v, ok := opts[k]; ok && v != nil {
			q.Set(k, fmt.Sprintf("%v", v))
		}
	}
	return "/api/widgets/resources?" + q.Encode()
}

// weatherURL builds the /api/widgets/openmeteo URL.
func weatherURL(opts map[string]interface{}) string {
	q := url.Values{}
	for _, k := range []string{"label", "latitude", "longitude", "timezone", "units", "cache"} {
		if v, ok := opts[k]; ok && v != nil {
			q.Set(k, fmt.Sprintf("%v", v))
		}
	}
	return "/api/widgets/openmeteo?" + q.Encode()
}

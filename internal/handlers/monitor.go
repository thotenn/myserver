package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
	"github.com/thotenn/myserver/internal/templates"
)

// SiteMonitor handles GET /api/siteMonitor.
// Performs an HTTP HEAD request (fallback to GET) and reports status +
// latency. Renders HTML for HTMX requests, JSON otherwise.
func SiteMonitor(w http.ResponseWriter, r *http.Request) {
	d, ok := dashboardOf(w, r)
	if !ok {
		return
	}
	targetURL := r.URL.Query().Get("url")
	if targetURL != "" {
		if !isAllowedSiteMonitor(d, targetURL) {
			writeJSONError(w, "url not allowed", http.StatusForbidden)
			return
		}
	} else {
		group := r.URL.Query().Get("groupName")
		service := r.URL.Query().Get("serviceName")
		if group != "" && service != "" {
			targetURL = resolveSiteMonitorURL(d, group, service)
		}
	}

	if targetURL == "" {
		writeJSONError(w, "missing url parameter", http.StatusBadRequest)
		return
	}

	// Each attempt has its own short timeout so we never exceed the
	// server's WriteTimeout (30s) with two sequential requests.
	headCtx, headCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer headCancel()
	start := time.Now()
	result, err := proxy.Proxy(headCtx, targetURL, &proxy.Params{
		Method:          http.MethodHead,
		FollowRedirects: true,
	})
	latency := time.Since(start).Milliseconds()

	if err != nil || result == nil || result.Status >= 400 {
		getCtx, getCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer getCancel()
		start = time.Now()
		result, err = proxy.Proxy(getCtx, targetURL, &proxy.Params{
			Method:          http.MethodGet,
			FollowRedirects: true,
		})
		latency = time.Since(start).Milliseconds()
	}

	status := 0
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	} else if result != nil {
		status = result.Status
	}

	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.SiteMonitorHTML(status, latency).Render(r.Context(), w)
		return
	}

	payload := map[string]interface{}{
		"status":  status,
		"latency": latency,
	}
	if errMsg != "" {
		payload["errors"] = []string{errMsg}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func resolveSiteMonitorURL(d *config.Dashboard, group, service string) string {
	groups, err := d.Services()
	if err != nil {
		return ""
	}
	for _, g := range groups {
		if g.Name != group {
			continue
		}
		for _, s := range g.Services {
			if strings.EqualFold(s.Name, service) {
				return s.SiteMonitor
			}
		}
	}
	return ""
}

// isAllowedSiteMonitor reports whether the given URL matches a siteMonitor
// field in a service of THIS dashboard. Same reason as isAllowedPingHost:
// unscoped, `?url=` reaches whatever any other dashboard listed.
func isAllowedSiteMonitor(d *config.Dashboard, target string) bool {
	groups, err := d.Services()
	if err != nil {
		return false
	}
	for _, g := range groups {
		for _, s := range g.Services {
			if s.SiteMonitor == target {
				return true
			}
		}
	}
	return false
}

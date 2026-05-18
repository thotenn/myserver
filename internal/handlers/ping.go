package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-ping/ping"
	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/templates"
)

// Ping handles GET /api/ping.
// Uses go-ping in unprivileged UDP mode (no CAP_NET_RAW needed). Renders
// HTML for HTMX requests, JSON otherwise.
//
// IMPORTANT: the JSON payload uses the key "errors" (not "error") because
// MyServer's customapi widget interprets a top-level "error" key as an API
// failure (see CLAUDE.md note).
func Ping(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host != "" {
		if !isAllowedPingHost(host) {
			writeJSONError(w, "host not allowed", http.StatusForbidden)
			return
		}
	} else {
		group := r.URL.Query().Get("groupName")
		service := r.URL.Query().Get("serviceName")
		if group != "" && service != "" {
			host = resolvePingHost(group, service)
		}
	}

	if host == "" {
		writeJSONError(w, "missing host parameter", http.StatusBadRequest)
		return
	}

	// Strip scheme/path so we ping just the hostname.
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Hostname()
	}

	pinger, err := ping.NewPinger(host)
	if err != nil {
		writePingResult(w, r, host, false, 0, err.Error())
		return
	}
	pinger.SetPrivileged(false)
	pinger.Count = 3
	pinger.Timeout = 5 * time.Second

	if err := pinger.Run(); err != nil {
		writePingResult(w, r, host, false, 0, err.Error())
		return
	}

	stats := pinger.Statistics()
	alive := stats.PacketsRecv > 0
	latency := stats.AvgRtt.Milliseconds()
	writePingResult(w, r, stats.Addr, alive, latency, "")
}

func writePingResult(w http.ResponseWriter, r *http.Request, host string, alive bool, latency int64, errMsg string) {
	if isHTMXRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.PingHTML(alive, latency).Render(r.Context(), w)
		return
	}
	payload := map[string]interface{}{
		"alive": alive,
		"host":  host,
		"time":  latency,
	}
	if errMsg != "" {
		// Use "errors" key (not "error") to avoid colliding with the
		// customapi widget's error semantics.
		payload["errors"] = []string{errMsg}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []string{msg},
	})
}

// resolvePingHost finds the ping URL for a service by looking it up in
// services.yaml.
func resolvePingHost(group, service string) string {
	groups, err := config.LoadServices()
	if err != nil {
		return ""
	}
	for _, g := range groups {
		if g.Name != group {
			continue
		}
		for _, s := range g.Services {
			if strings.EqualFold(s.Name, service) {
				return s.Ping
			}
		}
	}
	return ""
}

// isAllowedPingHost reports whether the given host matches a ping field in
// any configured service.
func isAllowedPingHost(target string) bool {
	groups, err := config.LoadServices()
	if err != nil {
		return false
	}
	for _, g := range groups {
		for _, s := range g.Services {
			if s.Ping == target {
				return true
			}
		}
	}
	return false
}

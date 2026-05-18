package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/thotenn/myserver/internal/config"
	mw "github.com/thotenn/myserver/internal/middleware"
	"github.com/thotenn/myserver/internal/scripts"
	"github.com/thotenn/myserver/internal/templates"
)

// ScriptManager is the global script manager instance.
// It is initialized in main.go if scripts are enabled.
var ScriptManager *scripts.Manager

// scriptsEnabled checks if the scripts feature is enabled.
func scriptsEnabled() bool {
	return config.ScriptsEnabled() && ScriptManager != nil
}

// ConfirmHeader is the HTTP header the frontend must send for scripts
// marked with `requireConfirm: true`. Its presence is the server-side
// enforcement of the confirmation — hx-confirm alone can be trivially
// bypassed by a malicious cross-site POST.
const ConfirmHeader = "X-Homepage-Confirm"

// ListScripts returns all registered scripts and their status.
func ListScripts(w http.ResponseWriter, r *http.Request) {
	if !scriptsEnabled() {
		http.NotFound(w, r)
		return
	}
	statuses := ScriptManager.List()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(statuses)
}

// GetScriptStatus returns the status of a specific script.
func GetScriptStatus(w http.ResponseWriter, r *http.Request) {
	if !scriptsEnabled() {
		http.NotFound(w, r)
		return
	}

	name := chi.URLParam(r, "name")
	status, err := ScriptManager.Status(name)
	if err != nil {
		http.Error(w, "script not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// RunScript executes a script and returns the result.
func RunScript(w http.ResponseWriter, r *http.Request) {
	if !scriptsEnabled() {
		http.NotFound(w, r)
		return
	}

	// CSRF defence: block obvious cross-origin requests by requiring either
	// a same-origin Origin header or an Origin absent (curl, etc).
	if !mw.IsOriginAllowed(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	name := chi.URLParam(r, "name")
	cfg, ok := ScriptManager.Get(name)
	if !ok {
		http.Error(w, "script not found", http.StatusNotFound)
		return
	}

	// Server-side enforcement of requireConfirm: client MUST opt-in via an
	// explicit header. The template sets this via HTMX hx-headers so the
	// legitimate flow still works without user-visible changes.
	if cfg.RequireConfirm && r.Header.Get(ConfirmHeader) != "yes" {
		http.Error(w, "confirmation required", http.StatusPreconditionRequired)
		return
	}

	clientIP := mw.ClientIPFromRequest(r)

	result, err := ScriptManager.Run(r.Context(), name, clientIP)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// If this request came from HTMX, render the ScriptResult partial so
	// the dashboard card updates in-place. Otherwise fall back to JSON for
	// API consumers.
	if isHTMXRequest(r) {
		svc := findServiceByScript(name)
		if svc == nil {
			svc = &config.Service{Name: cfg.Name, Script: name, Description: cfg.Description, Icon: cfg.Icon, RequireConfirm: cfg.RequireConfirm}
		}
		lang := preferredLang(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.ScriptResult(*svc, result.Status, result.ExitCode, result.Output, result.Duration.String(), lang).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// findServiceByScript looks up the Service entry in services.yaml for a
// given script name. Returns nil if no service references it (in which
// case the caller should fabricate a minimal Service).
func findServiceByScript(scriptName string) *config.Service {
	groups, err := config.LoadServices()
	if err != nil {
		return nil
	}
	for _, g := range groups {
		for i := range g.Services {
			if g.Services[i].Script == scriptName {
				s := g.Services[i]
				return &s
			}
		}
	}
	return nil
}

// preferredLang reads the language from the Settings. Falls back to "en".
func preferredLang(r *http.Request) string {
	s, err := config.LoadSettings()
	if err != nil || s == nil || s.Language == "" {
		return "en"
	}
	return s.Language
}

// StreamScript executes a script with SSE output streaming.
func StreamScript(w http.ResponseWriter, r *http.Request) {
	if !scriptsEnabled() {
		http.NotFound(w, r)
		return
	}
	if !mw.IsOriginAllowed(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	name := chi.URLParam(r, "name")
	cfg, ok := ScriptManager.Get(name)
	if !ok {
		http.Error(w, "script not found", http.StatusNotFound)
		return
	}
	if cfg.RequireConfirm && r.Header.Get(ConfirmHeader) != "yes" {
		http.Error(w, "confirmation required", http.StatusPreconditionRequired)
		return
	}

	clientIP := mw.ClientIPFromRequest(r)

	lines, err := ScriptManager.Stream(r.Context(), name, clientIP)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				_, _ = fmt.Fprintf(w, "event: done\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			// SSE: split on newlines and prefix each sub-line with
			// "data: " so embedded \n do not terminate the event.
			for _, sub := range strings.Split(strings.TrimRight(line, "\n"), "\n") {
				if _, err := fmt.Fprintf(w, "data: %s\n", sub); err != nil {
					return
				}
			}
			if _, err := fmt.Fprint(w, "\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

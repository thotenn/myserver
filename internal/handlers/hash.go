package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thotenn/myserver/internal/config"
)

// Hash returns the current config hash. Used by the frontend to poll for
// changes (hot-reload) and by the template to build cache-busting URLs.
func Hash(w http.ResponseWriter, r *http.Request) {
	hash := config.CurrentHash()
	if hash == "" {
		// Fallback: recompute on the fly on the first call before the
		// watcher has populated it.
		if h, err := config.ConfigHash(); err == nil {
			config.SetCurrentHash(h)
			hash = h
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"hash": hash})
}

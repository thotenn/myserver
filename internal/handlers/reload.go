package handlers

import (
	"encoding/json"
	"net/http"
)

func Reload(w http.ResponseWriter, r *http.Request) {
	// The config files are re-read on each request (no caching yet),
	// so reload is effectively a no-op at this stage.
	// When caching is implemented (Phase 2), this will clear caches.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reloaded"})
}

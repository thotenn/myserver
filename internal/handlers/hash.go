package handlers

import (
	"encoding/json"
	"net/http"
)

// Hash returns the current config hash. Used by the frontend to poll for
// changes (hot-reload) and by the template to build cache-busting URLs.
func Hash(w http.ResponseWriter, r *http.Request) {
	d, ok := dashboardOf(w, r)
	if !ok {
		return
	}
	// Per dashboard, on purpose: a change to one client's YAML must not make
	// every other dashboard's open browser reload.
	hash := d.Hash()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"hash": hash})
}

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thotenn/myserver/internal/config"
)

func Widgets(w http.ResponseWriter, r *http.Request) {
	d, ok := dashboardOf(w, r)
	if !ok {
		return
	}
	widgets, err := d.Widgets()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config.SanitizeWidgets(widgets))
}

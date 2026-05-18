package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thotenn/myserver/internal/config"
)

func Validate(w http.ResponseWriter, r *http.Request) {
	errors := []string{}

	// Validate services
	if _, err := config.LoadServices(); err != nil {
		errors = append(errors, "services.yaml: "+err.Error())
	}

	// Validate bookmarks
	if _, err := config.LoadBookmarks(); err != nil {
		errors = append(errors, "bookmarks.yaml: "+err.Error())
	}

	// Validate widgets
	if _, err := config.LoadWidgets(); err != nil {
		errors = append(errors, "widgets.yaml: "+err.Error())
	}

	// Validate settings
	if _, err := config.LoadSettings(); err != nil {
		errors = append(errors, "settings.yaml: "+err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	if len(errors) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":  false,
			"errors": errors,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid": true,
	})
}

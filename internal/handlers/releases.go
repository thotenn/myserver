package handlers

import (
	"encoding/json"
	"net/http"
)

func Releases(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement GitHub releases check with caching
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]interface{}{})
}

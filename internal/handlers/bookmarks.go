package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/thotenn/myserver/internal/config"
)

func Bookmarks(w http.ResponseWriter, r *http.Request) {
	bookmarks, err := config.LoadBookmarks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookmarks)
}

package handlers

import (
	"encoding/json"
	"net/http"
)

func Bookmarks(w http.ResponseWriter, r *http.Request) {
	d, ok := dashboardOf(w, r)
	if !ok {
		return
	}
	bookmarks, err := d.Bookmarks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookmarks)
}

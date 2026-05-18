package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/a-h/templ"
)

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// respondHTML renders a Templ component and writes it as text/html.
func respondHTML(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = component.Render(r.Context(), w)
}

// respond writes either JSON or HTML depending on the HX-Request header.
// If the request is from HTMX, it renders the HTML component; otherwise
// it returns the JSON payload.
func respond(w http.ResponseWriter, r *http.Request, html templ.Component, jsonPayload interface{}) {
	if isHTMXRequest(r) {
		respondHTML(w, r, html)
		return
	}
	respondJSON(w, http.StatusOK, jsonPayload)
}

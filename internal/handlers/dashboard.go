package handlers

import (
	"net/http"

	"github.com/thotenn/myserver/internal/config"
)

// dashboardOf returns the dashboard this request is being served for, or
// writes a 500 and reports false.
//
// Every handler in this package starts with it, and none of them has any other
// way to reach a config file. That is the point of the phase this came from:
// with several dashboards in one process, a handler that could ask for "the"
// config could serve one client's services to another, or — through the widget
// proxy — call an upstream with another client's API key. The dashboard is
// resolved once, at the edge, from the URL.
//
// middleware.Dispatch populates it in front of every router, so a missing one
// is a wiring bug rather than a request-shaped problem. It fails closed and
// loudly instead of falling back to anything.
func dashboardOf(w http.ResponseWriter, r *http.Request) (*config.Dashboard, bool) {
	d := config.DashboardFrom(r.Context())
	if d == nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return nil, false
	}
	return d, true
}

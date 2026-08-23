package middleware

import (
	"net/http"

	"github.com/thotenn/myserver/internal/config"
)

// Dispatch is the edge of the whole application. It decides which dashboard a
// request belongs to, strips that dashboard's URL prefix, publishes the
// dashboard on the request context, and hands the request to the router for
// that dashboard's surface.
//
// Stripping at the edge is the cheap half of the design, and it is what phase
// one bought: chi's route patterns, http.StripPrefix("/static/") and the auth
// gate's public-path allowlist keep matching the same paths they matched when
// there was one dashboard at the root, so a prefixed or tenanted deployment
// cannot develop a routing difference from the plain one. Only code that EMITS
// a URL knows about the prefix, via config.PrefixPathFrom.
//
// The other half is the context. Every handler reads its config through
// config.DashboardFrom(ctx), so "which dashboard" is answered exactly once,
// here, from the URL — and never again by reaching for a global.
//
// A path outside every prefix this process serves gets a 404: the instance
// owns the subtrees it was configured for and nothing else.
func Dispatch(root, client http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		set := config.Dashboards()
		if set == nil {
			// No registry means the process is not ready to say who owns this
			// path. Serving it from "the" config is the failure this design
			// exists to prevent.
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}

		d := set.Resolve(r.URL.Path)
		if d == nil {
			http.NotFound(w, r)
			return
		}

		next := root
		if !d.IsRoot() {
			next = client
		}

		if d.Prefix == "" {
			// Nothing to strip and nothing to clone beyond the context: an
			// unprefixed single-dashboard deployment reaches its router with
			// the same URL it arrived with.
			next.ServeHTTP(w, r.WithContext(config.WithDashboard(r.Context(), d)))
			return
		}

		rest, ok := config.StripPrefix(d.Prefix, r.URL.Path)
		if !ok {
			// Resolve matched the prefix, so this cannot happen; refusing is
			// still cheaper than serving a path we did not strip.
			http.NotFound(w, r)
			return
		}

		r2 := r.Clone(config.WithDashboard(r.Context(), d))
		r2.URL.Path = rest
		// RawPath is only set when the escaped form differs from Path. The
		// prefix itself never contains escapes (see config.ParseBasePath and
		// config.ValidateSlug), so the same literal is what has to come off.
		if r.URL.RawPath != "" {
			if raw, rawOK := config.StripPrefix(d.Prefix, r.URL.RawPath); rawOK {
				r2.URL.RawPath = raw
			} else {
				r2.URL.RawPath = ""
			}
		}
		next.ServeHTTP(w, r2)
	})
}

package middleware

import (
	"net/http"
	"strings"

	"github.com/thotenn/myserver/internal/config"
)

// BasePath serves the whole application under a URL prefix.
//
// It strips the prefix from the request before anything else sees it and puts
// it in the context instead. That is the cheap half of the design: chi's route
// patterns, http.StripPrefix("/static/") and the auth gate's public-path
// allowlist keep matching the same paths they match today, so a prefixed
// deployment cannot develop a routing difference from the unprefixed one. Only
// code that EMITS a URL has to know about the prefix, via
// config.PrefixPathFrom.
//
// Anything outside the prefix gets a 404: this instance owns exactly one
// subtree of the host.
func BasePath(prefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if prefix == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rest, ok := stripBasePath(prefix, r.URL.Path)
			if !ok {
				http.NotFound(w, r)
				return
			}

			r2 := r.Clone(config.WithBasePath(r.Context(), prefix))
			r2.URL.Path = rest
			// RawPath is only set when the escaped form differs from Path. The
			// prefix itself never contains escapes (see config.ParseBasePath),
			// so the same literal is what has to come off.
			if r.URL.RawPath != "" {
				if raw, rawOK := stripBasePath(prefix, r.URL.RawPath); rawOK {
					r2.URL.RawPath = raw
				} else {
					r2.URL.RawPath = ""
				}
			}
			next.ServeHTTP(w, r2)
		})
	}
}

// stripBasePath removes the prefix from a path, reporting whether the path was
// inside it. The bare prefix is the dashboard root: `/team` serves what `/`
// serves, with no redirect to `/team/`.
func stripBasePath(prefix, path string) (string, bool) {
	if path == prefix {
		return "/", true
	}
	if strings.HasPrefix(path, prefix+"/") {
		return path[len(prefix):], true
	}
	return "", false
}

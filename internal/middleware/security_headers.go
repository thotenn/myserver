package middleware

import (
	"net/http"
	"os"

	"github.com/thotenn/myserver/internal/config"
)

// SecurityHeaders adds security headers to every response.
//   - X-Frame-Options: DENY
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - X-Content-Type-Options: nosniff
//   - Content-Security-Policy (CSP)
//   - Strict-Transport-Security (HSTS, opt-in via HOMEPAGE_HSTS)
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// `img-src` is intentionally permissive on the protocol axis:
		//   - 'self'         → local files served via /api/config/... and /static/...
		//   - https:         → user-configured background images and custom
		//                       icon URLs hosted anywhere on the public web
		//                       (also covers cdn.jsdelivr.net / cdn.simpleicons.org)
		//   - data:          → inline favicons and embedded image data URIs
		// The dashboard sits behind an external auth layer in production,
		// so the threat surface for arbitrary image hosts is acceptable.
		csp := "default-src 'self'; " +
			"script-src 'self' unpkg.com cdn.jsdelivr.net; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' https: data:; " +
			"connect-src 'self';"
		// Only tightened when a login exists to protect: with authentication
		// off the header must stay byte-for-byte what it was before the
		// feature was added. The policy is per dashboard, so on a host serving
		// a public dashboard next to a gated one this is decided per request.
		// A request the edge could not resolve gets the tighter header.
		d := config.DashboardFrom(r.Context())
		if d == nil || d.Auth().Required {
			csp += " form-action 'self';"
		}
		w.Header().Set("Content-Security-Policy", csp)

		if os.Getenv("HOMEPAGE_HSTS") == "true" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

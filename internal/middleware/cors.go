package middleware

import (
	"net/http"
	"strings"
)

// CORS adds a restricted CORS policy for /api/* responses.
//
// Behaviour:
//   - If the request has an Origin header, it is reflected back ONLY when
//     its host matches the request Host header (same-origin policy).
//   - Cross-origin requests receive no Allow-Origin header, which causes
//     the browser to reject the response.
//   - Preflight OPTIONS requests are answered with the same origin rules.
//
// This is deliberately strict to limit the blast radius of cross-site POST
// requests to the script execution endpoints. It is NOT a substitute for
// real authentication in front of the dashboard.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isSameOrigin(origin, r.Host) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Homepage-Confirm, HX-Request, HX-Current-URL")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isSameOrigin returns true if the Origin header host matches the request
// Host header. Scheme is ignored because Host does not carry it.
func isSameOrigin(origin, host string) bool {
	o := origin
	if idx := strings.Index(o, "://"); idx >= 0 {
		o = o[idx+3:]
	}
	return strings.EqualFold(o, host)
}

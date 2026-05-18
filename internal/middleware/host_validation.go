package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/thotenn/myserver/internal/config"
	"go.uber.org/zap"
)

// HostValidation returns a middleware that validates the Host header against
// the list defined in HOMEPAGE_ALLOWED_HOSTS.
//
// Behaviour matches the Node.js Homepage middleware:
//   - When the env var is unset, ONLY localhost/loopback are allowed
//     (localhost:PORT, 127.0.0.1:PORT, [::1]:PORT). This is NOT allow-all.
//   - When the env var is "*", all hosts are allowed (opt-in).
//   - Otherwise the configured hosts are appended to the localhost defaults.
//   - Matching is case-insensitive and considers the full host:port string,
//     so HOMEPAGE_ALLOWED_HOSTS=example.com:3000 matches a Host header of
//     example.com:3000 (not bare example.com).
//
// Rejected requests are logged with the offending host so operators can
// diagnose configuration mistakes.
func HostValidation(port int, logger *zap.Logger) func(http.Handler) http.Handler {
	raw := config.RawAllowedHosts()
	allowAll := raw == "*"

	defaults := []string{
		fmt.Sprintf("localhost:%d", port),
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("[::1]:%d", port),
		// also accept the bare forms in case the client omits the port
		"localhost",
		"127.0.0.1",
		"[::1]",
	}

	hostSet := make(map[string]struct{}, len(defaults)+8)
	for _, h := range defaults {
		hostSet[strings.ToLower(h)] = struct{}{}
	}
	if raw != "" && !allowAll {
		for _, h := range strings.Split(raw, ",") {
			hostSet[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if allowAll {
				next.ServeHTTP(w, r)
				return
			}
			host := strings.ToLower(r.Host)
			if _, ok := hostSet[host]; ok {
				next.ServeHTTP(w, r)
				return
			}
			if logger != nil {
				logger.Warn("host validation rejected request",
					zap.String("host", r.Host),
					zap.String("remoteAddr", r.RemoteAddr),
					zap.String("path", r.URL.Path),
				)
			}
			http.Error(w, "Host validation failed. See server logs for details.", http.StatusBadRequest)
		})
	}
}

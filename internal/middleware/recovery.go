package middleware

import (
	"net/http"
	"runtime/debug"
	"strings"

	"go.uber.org/zap"
)

// Recovery recovers from panics in HTTP handlers, logs the stack trace,
// and returns a generic 500 to the client. The error body is matched to
// the response Content-Type so JSON clients receive JSON, HTML clients
// receive HTML, and others receive plain text.
func Recovery(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					if logger != nil {
						logger.Error("panic recovered",
							zap.Any("error", err),
							zap.String("path", r.URL.Path),
							zap.String("stack", string(debug.Stack())),
						)
					}
					ct := w.Header().Get("Content-Type")
					switch {
					case strings.Contains(ct, "application/json"):
						http.Error(w, `{"error":"internal_server_error"}`, http.StatusInternalServerError)
					case strings.Contains(ct, "text/html"):
						http.Error(w, "<h1>Internal Server Error</h1>", http.StatusInternalServerError)
					default:
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

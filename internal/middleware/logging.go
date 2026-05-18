package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Logging logs HTTP requests using zap.
func Logging(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			next.ServeHTTP(w, r)

			logger.Debug("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Duration("duration", time.Since(start)),
				zap.String("remote", ClientIPFromRequest(r)),
			)
		})
	}
}

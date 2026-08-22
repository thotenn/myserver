package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/thotenn/myserver/internal/config"
	mw "github.com/thotenn/myserver/internal/middleware"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// API sets up all routes and returns an http.Handler.
// Pass the listening port so that HostValidation can build localhost defaults.
func API(logger *zap.Logger, port int) http.Handler {
	r := chi.NewRouter()
	setupMiddleware(r, logger)
	setupRoutes(r, logger, port)
	return r
}

// setupMiddleware attaches global and group-level middleware.
func setupMiddleware(r chi.Router, logger *zap.Logger) {
	// Global middleware (applied to everything, including static files).
	r.Use(mw.Recovery(logger))
	r.Use(mw.Logging(logger))
	r.Use(mw.SecurityHeaders)
	// The auth gate sits above every route, including static files and the
	// dashboard itself. With an empty allowlist it is a single bool check and
	// passes the request straight through.
	r.Use(mw.Auth(logger))
	r.Use(middleware.Compress(5, "text/html", "text/css", "application/javascript", "application/json"))
}

// setupRoutes registers all application routes.
func setupRoutes(r chi.Router, logger *zap.Logger, port int) {
	// Static files (CSS, JS, locales) with immutable cache headers.
	fs := http.StripPrefix("/static/", http.FileServer(http.Dir("web/static")))
	r.Handle("/static/*", cacheControl(fs, "public, max-age=86400"))

	// Main dashboard page
	r.Get("/", Dashboard())

	// Authentication. These routes are always registered and answer 404 while
	// the allowlist is empty, so that filling in auth.yaml arms the login
	// without a restart (see internal/handlers/auth.go).
	r.Route("/auth", func(r chi.Router) {
		r.Get("/login", AuthLogin(logger))
		r.Get("/denied", AuthDenied(logger))
		// The login endpoints are the one unauthenticated path that talks to
		// an upstream, so they get the tightest rate limit in the app.
		r.With(rateLimit(1, 3)).Get("/google/start", AuthGoogleStart(logger))
		r.With(rateLimit(1, 3)).Get("/google/callback", AuthGoogleCallback(logger))
		r.Post("/logout", AuthLogout(logger))
	})

	// API routes with CORS, host validation, and rate limiting.
	// Apply a restricted CORS policy only to /api/* so the dashboard HTML is
	// NOT served with wildcard CORS, limiting the blast radius of cross-site
	// POSTs to the scripts endpoints.
	r.Route("/api", func(r chi.Router) {
		r.Use(mw.CORS)
		r.Use(mw.HostValidation(port, logger))

		// Hash endpoint — 1 req/s per IP
		r.With(rateLimit(1, 1)).Get("/hash", Hash)

		r.Get("/healthcheck", HealthCheck)
		r.Get("/services", Services)
		r.Get("/bookmarks", Bookmarks)
		r.Get("/widgets", Widgets)
		r.Get("/config/{path}", ConfigFile)
		r.Post("/reload", Reload)
		r.Get("/validate", Validate(logger))

		// Stubs — disabled until real implementation
		// r.Get("/releases", Releases)
		// r.Get("/search/searchSuggestion", SearchSuggestion)

		// Proxy — 10 req/s per IP
		r.With(rateLimit(10, 15)).Get("/services/proxy", Proxy)
		r.With(rateLimit(10, 15)).Post("/services/proxy", Proxy)

		// Docker
		r.Get("/docker/stats/{container}/{server}", DockerStats)
		r.Get("/docker/status/{container}/{server}", DockerStatus)

		// Kubernetes — stubs, disabled until real implementation
		// r.Get("/kubernetes/stats/{service}/{server}", KubernetesStats)
		// r.Get("/kubernetes/status/{service}/{server}", KubernetesStatus)

		// Proxmox
		r.Get("/proxmox/stats/{vmid}/{server}", ProxmoxStats)

		// Monitoring — ping 5 req/s per IP
		r.With(rateLimit(5, 8)).Get("/ping", Ping)
		r.With(rateLimit(5, 8)).Get("/siteMonitor", SiteMonitor)

		// Info widgets with data endpoints
		r.Get("/widgets/resources", ResourcesWidget)
		r.Get("/widgets/openmeteo", OpenMeteoWidget)
		r.Get("/widgets/weather", OpenMeteoWidget)

		// Scripts — only register when enabled so disabled deployments have
		// zero attack surface (the handlers still check internally for
		// defence in depth). Rate limit: 1 req/s per IP.
		if config.ScriptsEnabled() {
			r.With(rateLimit(1, 2)).Get("/scripts", ListScripts)
			r.With(rateLimit(1, 2)).Get("/scripts/{name}/status", GetScriptStatus)
			r.With(rateLimit(1, 2)).Post("/scripts/{name}", RunScript)
			r.With(rateLimit(1, 2)).Post("/scripts/{name}/stream", StreamScript)
		}
	})
}

// rateLimit is a convenience wrapper for the middleware rate limiter.
func rateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	return mw.RateLimiter(func(ip string) *rate.Limiter {
		return mw.NewRateLimiter(rps, burst)
	})
}

// cacheControl wraps a handler with a fixed Cache-Control header.
func cacheControl(h http.Handler, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		h.ServeHTTP(w, r)
	})
}

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/thotenn/myserver/internal/auth"
	"github.com/thotenn/myserver/internal/config"
	"go.uber.org/zap"
)

// LoginPath is where unauthenticated browsers are sent. Like every path inside
// a handler it is relative to the dashboard: the base path is added when the
// redirect is emitted, by config.PrefixPathFrom.
const LoginPath = "/auth/login"

// DeniedPath is where an authenticated caller who is not on the allowlist goes.
const DeniedPath = "/auth/denied"

// contextKey is unexported so no other package can collide with it.
type contextKey struct{ name string }

var sessionEmailKey = &contextKey{"auth.email"}

// SessionEmail returns the authenticated email attached to a request, or ""
// when authentication is off or the request is anonymous.
func SessionEmail(ctx context.Context) string {
	email, _ := ctx.Value(sessionEmailKey).(string)
	return email
}

// publicPrefixes are served without a session even when auth is on.
//
// This is an allowlist, not a denylist, and that is the whole point: /api/services,
// /api/widgets and /api/services/proxy together rebuild the dashboard from
// outside, and /api/scripts/* runs shell commands. Protecting only "/" would
// protect nothing. A route added tomorrow is protected by default.
var publicPrefixes = []string{
	"/static/", // the login page needs its CSS
	"/auth/",   // the login flow itself
}

// publicExact are single public paths.
var publicExact = map[string]bool{
	// The compose healthcheck calls this without credentials.
	"/api/healthcheck": true,
}

// Auth gates every request behind the email allowlist when auth.yaml asks for
// it. When it does not, the handler chain is untouched: no cookie is read, no
// header is written, nothing is redirected.
func Auth(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The dashboard comes from the edge, which resolved it from the
			// URL. Without one there is no policy to enforce and no safe
			// default: refusing is the only answer that cannot serve one
			// tenant's content under another's rules.
			d := config.DashboardFrom(r.Context())
			if d == nil {
				lockdown(w, r, logger, config.AuthState{
					Lockdown: true,
					Err:      errors.New("no dashboard on the request context"),
				})
				return
			}

			// Read the policy per request, never captured in this closure:
			// that is what makes the allowlist hot-reloadable, so removing an
			// address from the YAML evicts that person on their next request.
			state := d.Auth()

			if state.Lockdown {
				lockdown(w, r, logger, state)
				return
			}
			if !state.Required {
				// Belt and braces: a deployment that can never afford to be
				// public opts into failing closed even when the policy says
				// there is nothing to enforce.
				if config.AuthRequiredEnv() {
					lockdown(w, r, logger, state)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			if isPublicPath(r.URL.Path, state.Config) {
				next.ServeHTTP(w, r)
				return
			}

			email, ok := authenticate(w, r, d, state.Config)
			if !ok {
				challenge(w, r)
				return
			}

			// The allowlist is re-checked here on every request, not only at
			// login: a still-valid cookie is not a standing permission.
			if !auth.IsAllowed(state.Config, email) {
				logger.Warn("access denied: email not in allowlist",
					zap.String("email", email),
					zap.String("ip", ClientIPFromRequest(r)),
					zap.String("path", r.URL.Path))
				forbidden(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), sessionEmailKey, email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// authenticate resolves the caller's identity for the configured provider.
func authenticate(w http.ResponseWriter, r *http.Request, d *config.Dashboard, cfg *config.AuthConfig) (string, bool) {
	if cfg.ProviderName() == config.ProviderTrustedHeader {
		email, err := auth.EmailFromTrustedHeader(r, cfg, PeerIsTrusted(r))
		if err != nil {
			return "", false
		}
		return email, true
	}

	session, err := auth.ReadSession(r, d, cfg)
	if err != nil {
		return "", false
	}
	auth.MaybeRenewSession(w, d, cfg, session)
	return session.Email, true
}

// isPublicPath reports whether a path is served without a session.
func isPublicPath(path string, cfg *config.AuthConfig) bool {
	if publicExact[path] {
		return true
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	if cfg != nil {
		for _, p := range cfg.PublicPaths {
			if path == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
				return true
			}
		}
	}
	return false
}

// challenge answers an unauthenticated request in the form its caller can act
// on.
func challenge(w http.ResponseWriter, r *http.Request) {
	// Emitted URLs, unlike matched ones, carry the base path of the dashboard
	// this request is being served under.
	loginPath := config.PrefixPathFrom(r.Context(), LoginPath)
	// HTMX polls widgets in the background. Answering those with the login
	// HTML would make HTMX paint the login page inside a widget card; the
	// HX-Redirect header navigates the whole page instead.
	//
	// No `next` here on purpose: these requests are for data fragments, and
	// returning the visitor to one after signing in would leave them looking
	// at a bare widget instead of the dashboard.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", loginPath)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// `next` stays dashboard-relative: r.URL has already had the prefix
	// stripped by middleware.BasePath, and every redirect that later consumes
	// it adds the prefix back. That is what confines a caller-supplied
	// destination to this dashboard without a second validation rule.
	target := loginPath
	if next := r.URL.RequestURI(); next != "" && next != "/" {
		target += "?next=" + url.QueryEscape(next)
	}
	if wantsJSON(r) {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// forbidden answers a caller who authenticated but is not on the allowlist.
func forbidden(w http.ResponseWriter, r *http.Request) {
	deniedPath := config.PrefixPathFrom(r.Context(), DeniedPath)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", deniedPath)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if wantsJSON(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	http.Redirect(w, r, deniedPath, http.StatusFound)
}

// lockdown refuses to serve anything while the auth policy is unknown.
// Answering 503 is the whole safety property of this feature: a config we
// cannot read must never be treated as "no auth configured".
func lockdown(w http.ResponseWriter, r *http.Request, logger *zap.Logger, state config.AuthState) {
	if state.Err != nil {
		logger.Error("auth policy unavailable, refusing requests", zap.Error(state.Err))
	}
	// The healthcheck stays answerable so the container is reported unhealthy
	// rather than silently restarting in a loop.
	if r.URL.Path == "/api/healthcheck" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "auth policy unavailable"})
		return
	}
	if wantsJSON(r) {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}
	http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
}

// wantsJSON reports whether the caller is an API client rather than a browser.
func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return true
	}
	// An API path with no HTML preference is a programmatic caller.
	return strings.HasPrefix(r.URL.Path, "/api/") && !strings.Contains(accept, "text/html")
}

// writeJSONError emits a generic error body. Details go to the log, never to
// the client.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

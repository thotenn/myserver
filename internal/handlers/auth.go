package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/thotenn/myserver/internal/auth"
	"github.com/thotenn/myserver/internal/config"
	mw "github.com/thotenn/myserver/internal/middleware"
	"github.com/thotenn/myserver/internal/templates"
	"go.uber.org/zap"
)

// The /auth/* routes are always registered, but every handler answers 404
// while the allowlist is empty — byte for byte what chi returns for an
// unknown route today.
//
// Registering them conditionally would have been closer to how the scripts
// feature works, but scripts are switched on by an environment variable
// (which needs a restart anyway) whereas the allowlist is edited live. With
// conditional routes, adding the first email to auth.yaml would arm the gate
// while /auth/login still 404s, locking the operator out of their own
// dashboard until they restarted the process.

// oauthStateCookie carries state, nonce and the post-login destination
// between /auth/google/start and the callback.
//
// The __Host- prefix is not decoration: it makes the browser refuse the
// cookie unless it is Secure, Path=/ and free of a Domain attribute, so a
// sibling subdomain cannot plant one. It requires HTTPS, so plain-http local
// development falls back to the unprefixed name.
//
// __Host- also REQUIRES Path=/, so this cookie cannot be scoped to a base
// path the way the session cookie is. Two dashboards on the same host are
// therefore separated by the cookie NAME instead: the base path is appended,
// so a login in progress under one prefix cannot overwrite the state of a
// login under another. With no base path the name is unchanged.
//
// The name is a bucket, not an identity. What the callback trusts is the
// SIGNED payload inside, which names the dashboard the login belongs to — see
// oauthState and internal/auth/state.go. A shared callback receives every
// Path=/ cookie the browser holds and has to pick the flow it is completing
// out of them, which it does by matching the state; the name only keeps two
// concurrent logins from overwriting each other.
const (
	oauthStateCookieSecure = "__Host-myserver_oauth"
	oauthStateCookiePlain  = "myserver_oauth"
	oauthStateMaxAge       = 600 // 10 minutes
)

// redirect sends the browser to a dashboard-relative path, prefixed with the
// base path this request is served under. Every redirect in this file goes
// through it: a bare http.Redirect to "/auth/login" would leave a prefixed
// deployment bouncing to the host root.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, config.PrefixPathFrom(r.Context(), path), http.StatusFound)
}

// redirectTo is redirect for a dashboard that is NOT the one serving the
// request. Only the shared OAuth callback needs it: it runs on the root
// dashboard's route while completing a login that belongs to a client's.
func redirectTo(w http.ResponseWriter, r *http.Request, d *config.Dashboard, path string) {
	http.Redirect(w, r, d.PrefixPath(path), http.StatusFound)
}

// AuthLogin renders the login page.
func AuthLogin(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := dashboardOf(w, r)
		if !ok {
			return
		}
		state := d.Auth()
		if !state.Required {
			http.NotFound(w, r)
			return
		}
		// Already signed in and still allowed? Nothing to do here.
		if session, err := auth.ReadSession(r, d, state.Config); err == nil &&
			auth.IsAllowed(state.Config, session.Email) {
			redirect(w, r, "/")
			return
		}

		data := authPageData(d)
		data.StartURL = config.PrefixPathFrom(r.Context(), "/auth/google/start") +
			nextQuery(safeNext(r.URL.Query().Get("next")))
		data.Error = errorKey(r.URL.Query().Get("error"))
		w.Header().Set("Cache-Control", "no-store")
		respondHTML(w, r, templates.LoginPage(data))
	}
}

// AuthDenied renders the "your address is not listed" page.
func AuthDenied(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := dashboardOf(w, r)
		if !ok {
			return
		}
		state := d.Auth()
		if !state.Required {
			http.NotFound(w, r)
			return
		}
		data := authPageData(d)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		respondHTML(w, r, templates.DeniedPage(data))
	}
}

// AuthGoogleStart redirects to Google with a fresh state and nonce.
func AuthGoogleStart(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := dashboardOf(w, r)
		if !ok {
			return
		}
		state := d.Auth()
		if !state.Required || state.Config.ProviderName() != config.ProviderGoogle {
			http.NotFound(w, r)
			return
		}

		oauthState, err := randomToken()
		if err != nil {
			logger.Error("failed to generate oauth state", zap.Error(err))
			redirect(w, r, "/auth/login?error=failed")
			return
		}
		nonce, err := randomToken()
		if err != nil {
			logger.Error("failed to generate oauth nonce", zap.Error(err))
			redirect(w, r, "/auth/login?error=failed")
			return
		}

		next := safeNext(r.URL.Query().Get("next"))
		setOAuthStateCookie(w, d, state.Config, oauthState, nonce, next)
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, auth.AuthorizationURL(state.Config, oauthState, nonce), http.StatusFound)
	}
}

// AuthGoogleCallback completes the login.
func AuthGoogleCallback(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		here, ok := dashboardOf(w, r)
		if !ok {
			return
		}
		// Cache-Control is set per branch rather than up front: the 404 this
		// route returns when no login exists has to stay byte for byte the 404
		// chi returns for an unknown route.
		noStore := func() { w.Header().Set("Cache-Control", "no-store") }

		// One callback completes logins for every dashboard that points its
		// redirectURL at it, which is what removes the per-client entry in the
		// identity provider's console — the operational cost this whole
		// feature exists to delete.
		//
		// So the dashboard being completed is NOT the one serving this
		// request: it is named in the signed state, and it is found by
		// matching the state value against the cookies the browser sent.
		stored, cookieName, inFlight := matchOAuthStateCookie(r)
		if stored == nil {
			if inFlight > 0 {
				// A login IS in progress, but the state Google echoed back is
				// not the one that started it. That is CSRF on the login flow,
				// and it is answered the same way it was before there were
				// several dashboards.
				noStore()
				clearOAuthStateCookieNamed(w, cookieName, here.Auth().Config)
				logger.Warn("oauth state mismatch", zap.String("ip", mw.ClientIPFromRequest(r)))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			// Nothing in flight. With no login configured on the dashboard
			// serving this route, the route does not exist — which is what
			// keeps an auth-less deployment byte-for-byte what it was.
			hereState := here.Auth()
			if !hereState.Required || hereState.Config.ProviderName() != config.ProviderGoogle {
				http.NotFound(w, r)
				return
			}
			noStore()
			clearOAuthStateCookie(w, here, hereState.Config)
			logger.Warn("oauth callback without a matching state cookie",
				zap.String("ip", mw.ClientIPFromRequest(r)))
			redirect(w, r, "/auth/login?error=expired")
			return
		}

		// The slug is read from the signed payload and never from the query.
		// It decides which allowlist the login is checked against, so a caller
		// able to choose it would be choosing the policy that judges them.
		d := config.Dashboards().Client(stored.Dashboard)
		if stored.Dashboard == "" {
			d = config.Dashboards().Root()
		}
		if d == nil {
			noStore()
			clearOAuthStateCookieNamed(w, cookieName, here.Auth().Config)
			logger.Warn("oauth callback for a dashboard that no longer exists",
				zap.String("dashboard", stored.Dashboard))
			redirect(w, r, "/auth/login?error=failed")
			return
		}
		state := d.Auth()
		if !state.Required || state.Config.ProviderName() != config.ProviderGoogle {
			http.NotFound(w, r)
			return
		}
		noStore()
		clearOAuthStateCookieNamed(w, cookieName, state.Config)
		cfg := state.Config

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			logger.Warn("oauth provider returned an error", zap.String("error", errParam))
			redirectTo(w, r, d, "/auth/login?error=failed")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			redirectTo(w, r, d, "/auth/login?error=failed")
			return
		}

		email, err := auth.ExchangeCode(r.Context(), cfg, code, stored.Nonce)
		if err != nil {
			// The upstream error can name the token endpoint and the client
			// id, so it stays in the log and never reaches the browser.
			logger.Warn("oauth code exchange failed",
				zap.Error(err), zap.String("ip", mw.ClientIPFromRequest(r)))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !auth.IsAllowed(cfg, email) {
			logger.Warn("login denied: email not in allowlist",
				zap.String("email", email),
				zap.String("ip", mw.ClientIPFromRequest(r)))
			// No cookie is issued: authenticating with Google proves who you
			// are, not that you are welcome.
			redirectTo(w, r, d, "/auth/denied")
			return
		}

		if err := auth.IssueSession(w, d, cfg, email); err != nil {
			logger.Error("failed to issue session", zap.Error(err))
			redirectTo(w, r, d, "/auth/login?error=failed")
			return
		}
		logger.Info("login succeeded",
			zap.String("email", email),
			zap.String("dashboard", d.String()),
			zap.String("ip", mw.ClientIPFromRequest(r)))

		// Re-validate the destination at the point of use, not only when it
		// was stored. safeNext keeps it dashboard-relative; redirectTo puts it
		// back under the prefix of the dashboard that was signed into, which is
		// what makes a stored destination unable to escape to another one.
		redirectTo(w, r, d, safeNext(stored.Next))
	}
}

// AuthLogout clears the session.
func AuthLogout(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := dashboardOf(w, r)
		if !ok {
			return
		}
		state := d.Auth()
		if !state.Required {
			http.NotFound(w, r)
			return
		}
		auth.ClearSession(w, d, state.Config)
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", config.PrefixPathFrom(r.Context(), "/auth/login"))
			w.WriteHeader(http.StatusOK)
			return
		}
		redirect(w, r, "/auth/login")
	}
}

// oauthState is the short-lived state kept in the cookie during the redirect
// to Google and back. It travels signed (internal/auth/state.go).
type oauthState struct {
	State string `json:"s"`
	Nonce string `json:"n"`
	Next  string `json:"x"`
	// Dashboard is the slug of the dashboard this login belongs to, "" for the
	// root one. It is the field the shared callback exists for, and the reason
	// the payload is signed: it selects the allowlist the login is judged by.
	Dashboard string `json:"d,omitempty"`
}

func oauthCookieName(d *config.Dashboard, cfg *config.AuthConfig) string {
	return oauthCookieBase(cfg) + oauthCookieSuffix(d.Prefix)
}

// oauthCookieBase is the name without the per-dashboard suffix, and the prefix
// the callback scans the request's cookies for.
func oauthCookieBase(cfg *config.AuthConfig) string {
	if cfg.CookieSecure() {
		return oauthStateCookieSecure
	}
	return oauthStateCookiePlain
}

// oauthCookieSuffix turns a base path into a cookie-name suffix. The prefix
// charset (config.ParseBasePath) is already a subset of what a cookie name
// allows, so only the separators change.
func oauthCookieSuffix(prefix string) string {
	if prefix == "" {
		return ""
	}
	return "_" + strings.ReplaceAll(strings.Trim(prefix, "/"), "/", "_")
}

func setOAuthStateCookie(w http.ResponseWriter, d *config.Dashboard, cfg *config.AuthConfig, state, nonce, next string) {
	payload, err := json.Marshal(oauthState{
		State: state, Nonce: nonce, Next: next, Dashboard: d.Slug,
	})
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthCookieName(d, cfg),
		Value:    auth.SealState(payload),
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   oauthStateMaxAge,
	})
}

// matchOAuthStateCookie finds the in-flight login this callback is completing.
// It returns the matching state and its cookie name, plus how many valid
// in-flight logins the request carried — which is what tells "the state does
// not match" (CSRF) apart from "there was no login to complete" (expired).
//
// A shared callback cannot ask for a cookie by name: it does not yet know
// which dashboard the flow belongs to, and that is precisely what the cookie
// is carrying. So it looks at every OAuth state cookie the browser sent (they
// are all Path=/), keeps the ones whose signature verifies, and picks the one
// whose state matches the one the provider echoed back. The comparison is
// constant-time: the state is a secret shared between the cookie and the
// redirect, and comparing it byte-wise would leak it.
func matchOAuthStateCookie(r *http.Request) (*oauthState, string, int) {
	want := r.URL.Query().Get("state")
	inFlight := 0
	lastName := ""
	for _, c := range r.Cookies() {
		if !strings.HasPrefix(c.Name, oauthStateCookieSecure) &&
			!strings.HasPrefix(c.Name, oauthStateCookiePlain) {
			continue
		}
		raw, ok := auth.OpenState(c.Value)
		if !ok {
			continue
		}
		var s oauthState
		if err := json.Unmarshal(raw, &s); err != nil || s.State == "" || s.Nonce == "" {
			continue
		}
		inFlight++
		lastName = c.Name
		if want != "" && subtle.ConstantTimeCompare([]byte(want), []byte(s.State)) == 1 {
			return &s, c.Name, inFlight
		}
	}
	return nil, lastName, inFlight
}

func clearOAuthStateCookie(w http.ResponseWriter, d *config.Dashboard, cfg *config.AuthConfig) {
	clearOAuthStateCookieNamed(w, oauthCookieName(d, cfg), cfg)
}

func clearOAuthStateCookieNamed(w http.ResponseWriter, name string, cfg *config.AuthConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// randomToken returns 32 bytes of entropy, URL-safe.
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// safeNext keeps only same-site destinations.
//
// "//evil.com" and "/\evil.com" are both read by browsers as protocol-relative
// URLs pointing off-site, which is why a bare HasPrefix(next, "/") is not
// enough to stop an open redirect.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") {
		return "/"
	}
	if strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

// nextQuery renders the ?next= suffix for a destination worth preserving.
func nextQuery(next string) string {
	if next == "" || next == "/" {
		return ""
	}
	return "?next=" + url.QueryEscape(next)
}

// errorKey maps a query parameter to a translation key, so no attacker-chosen
// text is ever rendered on the login page.
func errorKey(param string) string {
	switch param {
	case "failed":
		return "auth.error.failed"
	case "expired":
		return "auth.error.expired"
	default:
		return ""
	}
}

// authPageData assembles what the auth pages render, taking the dashboard
// title and language from settings when they are readable.
func authPageData(d *config.Dashboard) templates.AuthPageData {
	data := templates.AuthPageData{
		Title:        "MyServer",
		Language:     "en",
		Theme:        "dark",
		Color:        "slate",
		AssetVersion: AssetVersion(),
	}
	if settings, err := d.Settings(); err == nil && settings != nil {
		if settings.Title != "" {
			data.Title = settings.Title
		}
		if settings.Language != "" {
			data.Language = settings.Language
		}
		if settings.Theme != "" {
			data.Theme = settings.Theme
		}
		if settings.Color != "" {
			data.Color = settings.Color
		}
	}
	return data
}

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
const (
	oauthStateCookieSecure = "__Host-myserver_oauth"
	oauthStateCookiePlain  = "myserver_oauth"
	oauthStateMaxAge       = 600 // 10 minutes
)

// AuthLogin renders the login page.
func AuthLogin(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := config.Auth()
		if !state.Required {
			http.NotFound(w, r)
			return
		}
		// Already signed in and still allowed? Nothing to do here.
		if session, err := auth.ReadSession(r, state.Config); err == nil &&
			auth.IsAllowed(state.Config, session.Email) {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		data := authPageData(state.Config)
		data.StartURL = "/auth/google/start" + nextQuery(safeNext(r.URL.Query().Get("next")))
		data.Error = errorKey(r.URL.Query().Get("error"))
		w.Header().Set("Cache-Control", "no-store")
		respondHTML(w, r, templates.LoginPage(data))
	}
}

// AuthDenied renders the "your address is not listed" page.
func AuthDenied(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := config.Auth()
		if !state.Required {
			http.NotFound(w, r)
			return
		}
		data := authPageData(state.Config)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		respondHTML(w, r, templates.DeniedPage(data))
	}
}

// AuthGoogleStart redirects to Google with a fresh state and nonce.
func AuthGoogleStart(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := config.Auth()
		if !state.Required || state.Config.ProviderName() != config.ProviderGoogle {
			http.NotFound(w, r)
			return
		}

		oauthState, err := randomToken()
		if err != nil {
			logger.Error("failed to generate oauth state", zap.Error(err))
			http.Redirect(w, r, "/auth/login?error=failed", http.StatusFound)
			return
		}
		nonce, err := randomToken()
		if err != nil {
			logger.Error("failed to generate oauth nonce", zap.Error(err))
			http.Redirect(w, r, "/auth/login?error=failed", http.StatusFound)
			return
		}

		next := safeNext(r.URL.Query().Get("next"))
		setOAuthStateCookie(w, state.Config, oauthState, nonce, next)
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, auth.AuthorizationURL(state.Config, oauthState, nonce), http.StatusFound)
	}
}

// AuthGoogleCallback completes the login.
func AuthGoogleCallback(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := config.Auth()
		if !state.Required || state.Config.ProviderName() != config.ProviderGoogle {
			http.NotFound(w, r)
			return
		}
		cfg := state.Config
		w.Header().Set("Cache-Control", "no-store")

		stored, ok := readOAuthStateCookie(r, cfg)
		clearOAuthStateCookie(w, cfg)
		if !ok {
			logger.Warn("oauth callback without a valid state cookie",
				zap.String("ip", mw.ClientIPFromRequest(r)))
			http.Redirect(w, r, "/auth/login?error=expired", http.StatusFound)
			return
		}

		// Constant-time: the state is a secret shared between the cookie and
		// the redirect, and comparing it byte-wise would leak it.
		got := r.URL.Query().Get("state")
		if subtle.ConstantTimeCompare([]byte(got), []byte(stored.State)) != 1 {
			logger.Warn("oauth state mismatch", zap.String("ip", mw.ClientIPFromRequest(r)))
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if errParam := r.URL.Query().Get("error"); errParam != "" {
			logger.Warn("oauth provider returned an error", zap.String("error", errParam))
			http.Redirect(w, r, "/auth/login?error=failed", http.StatusFound)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Redirect(w, r, "/auth/login?error=failed", http.StatusFound)
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
			http.Redirect(w, r, "/auth/denied", http.StatusFound)
			return
		}

		if err := auth.IssueSession(w, cfg, email); err != nil {
			logger.Error("failed to issue session", zap.Error(err))
			http.Redirect(w, r, "/auth/login?error=failed", http.StatusFound)
			return
		}
		logger.Info("login succeeded",
			zap.String("email", email),
			zap.String("ip", mw.ClientIPFromRequest(r)))

		// Re-validate the destination at the point of use, not only when it
		// was stored.
		http.Redirect(w, r, safeNext(stored.Next), http.StatusFound)
	}
}

// AuthLogout clears the session.
func AuthLogout(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := config.Auth()
		if !state.Required {
			http.NotFound(w, r)
			return
		}
		auth.ClearSession(w, state.Config)
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", "/auth/login")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/auth/login", http.StatusFound)
	}
}

// oauthState is the short-lived state kept in the cookie during the redirect
// to Google and back.
type oauthState struct {
	State string `json:"s"`
	Nonce string `json:"n"`
	Next  string `json:"x"`
}

func oauthCookieName(cfg *config.AuthConfig) string {
	if cfg.CookieSecure() {
		return oauthStateCookieSecure
	}
	return oauthStateCookiePlain
}

func setOAuthStateCookie(w http.ResponseWriter, cfg *config.AuthConfig, state, nonce, next string) {
	payload, err := json.Marshal(oauthState{State: state, Nonce: nonce, Next: next})
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthCookieName(cfg),
		Value:    base64.RawURLEncoding.EncodeToString(payload),
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.CookieSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   oauthStateMaxAge,
	})
}

func readOAuthStateCookie(r *http.Request, cfg *config.AuthConfig) (*oauthState, bool) {
	c, err := r.Cookie(oauthCookieName(cfg))
	if err != nil || c.Value == "" {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return nil, false
	}
	var s oauthState
	if err := json.Unmarshal(raw, &s); err != nil || s.State == "" || s.Nonce == "" {
		return nil, false
	}
	return &s, true
}

func clearOAuthStateCookie(w http.ResponseWriter, cfg *config.AuthConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthCookieName(cfg),
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
func authPageData(cfg *config.AuthConfig) templates.AuthPageData {
	data := templates.AuthPageData{
		Title:    "MyServer",
		Language: "en",
		Theme:    "dark",
		Color:    "slate",
		Hash:     config.CurrentHash(),
	}
	if settings, err := config.LoadSettings(); err == nil && settings != nil {
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

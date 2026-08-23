package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thotenn/myserver/internal/auth"
	"github.com/thotenn/myserver/internal/config"
	"go.uber.org/zap"
)

const testAuthYAML = `
allowlist:
  emails:
    - person@example.com
google:
  clientId: "client-id"
  clientSecret: "client-secret"
  redirectURL: "https://dashboard.example.com/auth/google/callback"
session:
  secret: "test-session-secret"
  secure: false
`

// authTestServices is a minimal dashboard so the protected endpoints have
// something to leak if the gate fails.
const authTestServices = `
- Apps:
    - Plex:
        href: https://plex.example.com
        widget:
          type: plex
          url: https://plex.example.com
          key: super-secret-key
`

// newAuthedRouter builds the real router over a config dir, with or without
// an auth.yaml, so these tests exercise the same middleware chain production
// does rather than a handler in isolation.
func newAuthedRouter(t *testing.T, withAuth bool) http.Handler {
	t.Helper()
	newAuthedDashboard(t, withAuth)
	return API(zap.NewNop(), 3000)
}

// newAuthedDashboard builds the root dashboard the router above serves. Each
// test gets a fresh one, which is also what keeps a lockdown left over from
// another case from being read here as "auth.yaml just disappeared": the
// policy belongs to the dashboard now, not to the package.
func newAuthedDashboard(t *testing.T, withAuth bool) *config.Dashboard {
	t.Helper()
	files := map[string]string{"services.yaml": authTestServices}
	if withAuth {
		files[config.AuthFile] = testAuthYAML
	}
	return withTempConfig(t, files)
}

// rootDashboard is the dashboard the current registry serves from the root.
func rootDashboard(t *testing.T) *config.Dashboard {
	t.Helper()
	set := config.Dashboards()
	require.NotNil(t, set, "no dashboard registry published")
	return set.Root()
}

// sessionCookie mints a valid session for the given email.
func sessionCookie(t *testing.T, email string) *http.Cookie {
	t.Helper()
	d := rootDashboard(t)
	cfg := d.Auth().Config
	require.NotNil(t, cfg, "auth must be configured")
	rec := httptest.NewRecorder()
	require.NoError(t, auth.IssueSession(rec, d, cfg, email))
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

// newRequest builds a request with a Host the API's HostValidation accepts,
// so these tests fail on the gate's behaviour rather than on host checking.
func newRequest(method, path string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.Host = "localhost:3000"
	return r
}

// protectedPaths are the endpoints that together rebuild the dashboard from
// the outside, plus the one that runs shell commands. Gating only "/" would
// gate nothing.
var protectedPaths = []string{
	"/",
	"/api/services",
	"/api/widgets",
	"/api/bookmarks",
	"/api/hash",
	"/api/validate",
	"/api/services/proxy?group=Apps&service=Plex&endpoint=default",
	"/api/config/custom.css",
	"/api/docker/stats/container/server",
	"/api/widgets/resources",
	"/api/ping?groupName=Apps&serviceName=Plex",
}

func TestAuth_GateCoversEveryContentPath(t *testing.T) {
	router := newAuthedRouter(t, true)

	for _, path := range protectedPaths {
		t.Run(path, func(t *testing.T) {
			req := newRequest(http.MethodGet, path)
			req.Header.Set("Accept", "text/html")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusFound, rec.Code, "anonymous browsers must be redirected to the login")
			assert.Equal(t, "/auth/login", locationPath(rec), "unexpected redirect target")
			assert.NotContains(t, rec.Body.String(), "super-secret-key", "content leaked to an anonymous caller")
		})
	}
}

func TestAuth_JSONClientsGet401(t *testing.T) {
	router := newAuthedRouter(t, true)

	for _, path := range []string{"/api/services", "/api/widgets", "/api/services/proxy?group=Apps&service=Plex"} {
		req := newRequest(http.MethodGet, path)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s should answer 401 to an API client", path)
		assert.NotContains(t, rec.Body.String(), "super-secret-key")
	}
}

// A widget polling in the background must navigate the page, not paint the
// login form inside its own card.
func TestAuth_HTMXGetsRedirectHeader(t *testing.T) {
	router := newAuthedRouter(t, true)

	req := newRequest(http.MethodGet, "/api/widgets/resources")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "/auth/login", rec.Header().Get("HX-Redirect"))
	assert.NotContains(t, rec.Body.String(), "<html", "HTMX would inject this markup into a widget")
}

func TestAuth_ValidSessionPassesThrough(t *testing.T) {
	router := newAuthedRouter(t, true)
	cookie := sessionCookie(t, "person@example.com")

	req := newRequest(http.MethodGet, "/api/services")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Plex")
}

// The property that makes the allowlist usable: it is re-checked per request,
// so removing somebody evicts them without waiting for their cookie to lapse.
func TestAuth_RemovingEmailEvictsOnNextRequest(t *testing.T) {
	router := newAuthedRouter(t, true)
	cookie := sessionCookie(t, "person@example.com")

	req := newRequest(http.MethodGet, "/api/services")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "precondition: the session works")

	// The operator edits auth.yaml and the watcher reloads.
	rewriteAuthFile(t, `
allowlist:
  emails:
    - someone-else@example.com
google:
  clientId: "client-id"
  clientSecret: "client-secret"
  redirectURL: "https://dashboard.example.com/auth/google/callback"
session:
  secret: "test-session-secret"
  secure: false
`)

	req = newRequest(http.MethodGet, "/api/services")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(cookie) // same, still cryptographically valid cookie
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"a valid cookie is not a standing permission; the allowlist decides on every request")
	assert.NotContains(t, rec.Body.String(), "super-secret-key")
}

func TestAuth_PublicPathsStayOpen(t *testing.T) {
	router := newAuthedRouter(t, true)

	// The compose healthcheck has no credentials to offer.
	req := newRequest(http.MethodGet, "/api/healthcheck")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "the container healthcheck must keep working")

	// The login page and its assets must be reachable, or nobody can log in.
	for _, path := range []string{"/auth/login", "/static/css/main.css"} {
		req := newRequest(http.MethodGet, path)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusFound, rec.Code, "%s must not redirect to the login", path)
	}
}

func TestAuth_ExtraPublicPathsFromConfig(t *testing.T) {
	router := newAuthedRouter(t, true)

	req := newRequest(http.MethodGet, "/api/config/custom.css")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code, "custom.css is operator content, gated by default")

	rewriteAuthFile(t, testAuthYAML+"\npublicPaths:\n  - /api/config/custom.css\n")

	req = newRequest(http.MethodGet, "/api/config/custom.css")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "publicPaths should open it explicitly")
}

// The headline promise of the feature: with no allowlist, nothing changes.
func TestAuth_DisabledLeavesResponsesUntouched(t *testing.T) {
	router := newAuthedRouter(t, false)

	for _, path := range protectedPaths {
		req := newRequest(http.MethodGet, path)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusFound, rec.Code, "%s must not redirect when auth is off", path)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "%s must not challenge when auth is off", path)
		assert.NotEqual(t, http.StatusServiceUnavailable, rec.Code, "%s must not lock down when auth is off", path)
		assert.Empty(t, rec.Result().Cookies(), "%s must not set a cookie when auth is off", path)
		assert.Empty(t, rec.Header().Get("HX-Redirect"))
	}
}

func TestAuth_DisabledKeepsCSPAndAuthRoutesAbsent(t *testing.T) {
	router := newAuthedRouter(t, false)

	req := newRequest(http.MethodGet, "/")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	assert.NotContains(t, csp, "form-action",
		"with auth off the CSP must stay byte-for-byte what it was before this feature")

	// The /auth/* routes exist in the router but must be invisible.
	for _, path := range []string{"/auth/login", "/auth/denied", "/auth/google/start", "/auth/google/callback"} {
		req := newRequest(http.MethodGet, path)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s must 404 while the allowlist is empty", path)
	}
}

func TestAuth_EnabledAddsFormActionToCSP(t *testing.T) {
	router := newAuthedRouter(t, true)

	req := newRequest(http.MethodGet, "/auth/login")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Contains(t, rec.Header().Get("Content-Security-Policy"), "form-action 'self'")
}

func TestAuth_LoginPageRenders(t *testing.T) {
	router := newAuthedRouter(t, true)

	req := newRequest(http.MethodGet, "/auth/login")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "/auth/google/start", "the login page needs a way to start the flow")
	assert.NotContains(t, body, "client-secret", "the client secret must never reach the browser")
	assert.NotContains(t, body, "/api/config/custom.css", "operator CSS sits behind the gate")
}

func TestAuth_StartRedirectsToGoogleWithStateCookie(t *testing.T) {
	router := newAuthedRouter(t, true)

	req := newRequest(http.MethodGet, "/auth/google/start?next=%2Fdashboard")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	location := rec.Header().Get("Location")
	assert.Contains(t, location, "https://accounts.google.com/o/oauth2/v2/auth")
	assert.NotContains(t, location, "client-secret")

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1, "the state and nonce must be pinned to the browser")
	assert.True(t, cookies[0].HttpOnly, "the state cookie must not be readable from JavaScript")
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
}

func TestAuth_CallbackRejectsStateMismatch(t *testing.T) {
	router := newAuthedRouter(t, true)

	// Obtain a legitimate state cookie, then answer with a different state.
	start := httptest.NewRecorder()
	router.ServeHTTP(start, newRequest(http.MethodGet, "/auth/google/start"))
	stateCookie := start.Result().Cookies()[0]

	req := newRequest(http.MethodGet, "/auth/google/callback?code=abc&state=forged")
	req.AddCookie(stateCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code, "a mismatched state is CSRF on the login flow")
	assert.Empty(t, sessionCookiesIn(rec), "no session may be issued")
}

func TestAuth_CallbackWithoutStateCookieIsRejected(t *testing.T) {
	router := newAuthedRouter(t, true)

	req := newRequest(http.MethodGet, "/auth/google/callback?code=abc&state=whatever")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/auth/login", locationPath(rec))
	assert.Empty(t, sessionCookiesIn(rec))
}

func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"/dashboard":       "/dashboard",
		"/a/b?c=d":         "/a/b?c=d",
		"":                 "/",
		"//evil.example":   "/", // protocol-relative: browsers navigate off-site
		"/\\evil.example":  "/", // the backslash variant browsers also accept
		"https://evil.com": "/",
		"javascript:alert": "/",
		"evil.example":     "/",
	}
	for in, want := range cases {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestErrorKeyOnlyAllowsKnownKeys(t *testing.T) {
	// The login page renders a translation key, never text from the URL.
	assert.Equal(t, "auth.error.failed", errorKey("failed"))
	assert.Equal(t, "auth.error.expired", errorKey("expired"))
	assert.Empty(t, errorKey("<script>alert(1)</script>"))
	assert.Empty(t, errorKey("anything else"))
}

// rewriteAuthFile edits auth.yaml in the active config dir and reloads, the
// way the fsnotify watcher does in production.
func rewriteAuthFile(t *testing.T, content string) {
	t.Helper()
	d := config.Dashboards().Root()
	require.NoError(t, os.WriteFile(filepath.Join(d.Dir, config.AuthFile), []byte(content), 0o600))
	d.Reload()
}

// locationPath returns the path of a redirect, dropping the query.
func locationPath(rec *httptest.ResponseRecorder) string {
	loc := rec.Header().Get("Location")
	for i, c := range loc {
		if c == '?' {
			return loc[:i]
		}
	}
	return loc
}

// sessionCookiesIn returns any session cookies the response tried to set.
func sessionCookiesIn(rec *httptest.ResponseRecorder) []*http.Cookie {
	var out []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		d := config.Dashboards().Root()
		if c.Name == d.CookieName(d.Auth().Config) && c.Value != "" {
			out = append(out, c)
		}
	}
	return out
}

// The scripts endpoints run shell commands, so they are the one place where a
// gate bypass would be more than an information leak.
func TestAuth_ScriptEndpointsGated(t *testing.T) {
	t.Setenv("HOMEPAGE_SCRIPTS_ENABLED", "true")
	router := newAuthedRouter(t, true)

	cases := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/scripts"},
		{http.MethodPost, "/api/scripts/restart-traefik"},
		{http.MethodPost, "/api/scripts/restart-traefik/stream"},
		{http.MethodGet, "/api/scripts/restart-traefik/status"},
	}
	for _, c := range cases {
		req := newRequest(c.method, c.path)
		req.Header.Set("Accept", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusNotFound}, rec.Code,
			"%s %s must never be reachable anonymously", c.method, c.path)
	}
}

// A policy that cannot be read must stop serving, not fall back to public.
func TestAuth_LockdownServesNothingButHealthcheck(t *testing.T) {
	router := newAuthedRouter(t, true)

	// The config directory goes away, as when a bind mount fails.
	d := rootDashboard(t)
	require.NoError(t, os.Remove(filepath.Join(d.Dir, config.AuthFile)))
	d.Reload()
	require.True(t, d.Auth().Lockdown, "precondition: the policy should be in lockdown")

	for _, path := range []string{"/", "/api/services", "/api/widgets"} {
		req := newRequest(http.MethodGet, path)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "%s must refuse to serve", path)
		assert.NotContains(t, rec.Body.String(), "super-secret-key")
	}

	// The healthcheck answers, so the platform sees an unhealthy container
	// instead of a hung one.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newRequest(http.MethodGet, "/api/healthcheck"))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestAuth_TrustedHeaderProvider(t *testing.T) {
	withTempConfig(t, map[string]string{
		"services.yaml": authTestServices,
		config.AuthFile: `
provider: trustedHeader
trustedHeader:
  header: "Cf-Access-Authenticated-User-Email"
allowlist:
  emails:
    - person@example.com
`,
	})
	router := API(zap.NewNop(), 3000)
	require.True(t, rootDashboard(t).Auth().Required)

	// RemoteAddr is 192.0.2.1 by default in httptest — not a trusted proxy,
	// so the header is just something the caller made up.
	req := newRequest(http.MethodGet, "/api/services")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cf-Access-Authenticated-User-Email", "person@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"an identity header from an untrusted peer must not authenticate anybody")
	assert.NotContains(t, rec.Body.String(), "super-secret-key")

	// Same header, this time from the loopback proxy in TRUSTED_PROXIES.
	req = newRequest(http.MethodGet, "/api/services")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cf-Access-Authenticated-User-Email", "person@example.com")
	req.RemoteAddr = "127.0.0.1:5555"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "a trusted proxy's assertion should be honoured")

	// Trusted peer, but the address is not on the allowlist.
	req = newRequest(http.MethodGet, "/api/services")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cf-Access-Authenticated-User-Email", "stranger@example.com")
	req.RemoteAddr = "127.0.0.1:5555"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code, "the allowlist still decides")
}

// /api/validate used to return the raw loader error, which wraps os.ReadFile
// and therefore carries the absolute config-dir path — handlers must not
// describe the container's filesystem layout.
func TestValidate_DoesNotLeakFilesystemPaths(t *testing.T) {
	// withTempConfig populates the snapshot exactly as a running process does
	// — Reload discards the load error and stores nil. A validation endpoint
	// reading from the snapshot would report this broken file as valid, which
	// is what makes this fixture resemble production.
	d := withTempConfig(t, map[string]string{
		// Invalid YAML: the loader fails and the error names the file.
		"services.yaml": "- Group:\n    - Svc:\n        href: [unclosed\n",
	})

	req := newRequest(http.MethodGet, "/api/validate")
	rec := httptest.NewRecorder()
	API(zap.NewNop(), 3000).ServeHTTP(rec, req)

	body := rec.Body.String()
	require.Equal(t, http.StatusBadRequest, rec.Code, "broken YAML should fail validation")

	assert.NotContains(t, body, d.Dir, "the absolute config path leaked: %s", body)
	assert.NotContains(t, body, "/app/config", "container layout leaked")
	// The useful part survives: the operator still learns which file is broken.
	assert.Contains(t, body, "services.yaml")
}

// The endpoint must read from disk. Backed by the cached loaders it answered
// `valid: true` for a file it had failed to parse, because ReloadCache stores
// nil and drops the error.
func TestValidate_ReportsBrokenFileEvenWithWarmCache(t *testing.T) {
	d := withTempConfig(t, map[string]string{
		"services.yaml": "- Group:\n    - Svc:\n        href: [unclosed\n",
	})

	services, err := d.Services()
	require.NoError(t, err, "precondition: the snapshot swallowed the parse error")
	require.Nil(t, services)

	req := newRequest(http.MethodGet, "/api/validate")
	rec := httptest.NewRecorder()
	API(zap.NewNop(), 3000).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"a broken services.yaml must not be reported as valid: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "services.yaml")
}

func TestScrubConfigPaths(t *testing.T) {
	cases := map[string]string{
		"reading config services.yaml: open /app/config/services.yaml: no such file": "reading config services.yaml: open services.yaml: no such file",
		"open /var/secrets/other.yaml: permission denied":                            "open other.yaml: permission denied",
		"yaml: line 12: did not find expected key":                                   "yaml: line 12: did not find expected key",
	}
	for in, want := range cases {
		if got := scrubConfigPaths("/app/config", in); got != want {
			t.Errorf("scrubConfigPaths(%q)\n  = %q\n want %q", in, got, want)
		}
	}
}

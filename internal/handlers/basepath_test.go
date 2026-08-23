package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thotenn/myserver/internal/auth"
	"github.com/thotenn/myserver/internal/config"
	"go.uber.org/zap"
)

// issueTestSession mints a session for a dashboard, which is what decides the
// cookie's name and Path.
func issueTestSession(w http.ResponseWriter, d *config.Dashboard, cfg *config.AuthConfig) error {
	return auth.IssueSession(w, d, cfg, "person@example.com")
}

const basePathTestServices = `
- Apps:
    - Plex:
        href: https://plex.example.com
        ping: plex.example.com
`

// newBasePathRouter builds the real router with (or without) a base path, so
// these tests exercise the same edge middleware production does.
//
// The prefix is memoised in config, hence the explicit reset on both sides:
// API() reads it once, at construction.
func newBasePathRouter(t *testing.T, prefix string, withAuth bool) http.Handler {
	t.Helper()
	// The prefix is memoised in config and read once, when the registry is
	// built — hence setting it before withTempConfig publishes one.
	t.Setenv("HOMEPAGE_BASE_PATH", prefix)
	config.ResetBasePath()

	files := map[string]string{"services.yaml": basePathTestServices}
	if withAuth {
		files[config.AuthFile] = testAuthYAML
	}
	withTempConfig(t, files)
	return API(zap.NewNop(), 3000)
}

func TestBasePath_ServesTheDashboardUnderThePrefix(t *testing.T) {
	router := newBasePathRouter(t, "/team", false)

	for _, path := range []string{"/team", "/team/", "/team/api/healthcheck", "/team/api/services"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, newRequest(http.MethodGet, path))
		assert.Equal(t, http.StatusOK, rec.Code, "%s must be served", path)
	}
}

func TestBasePath_EverythingOutsideThePrefixIs404(t *testing.T) {
	router := newBasePathRouter(t, "/team", false)

	// This instance owns exactly one subtree of the host. The root, a sibling
	// prefix, and a prefix that merely starts with the same characters are all
	// somebody else's problem.
	for _, path := range []string{"/", "/api/healthcheck", "/api/services", "/other/api/services", "/teamwork/api/services", "/static/css/main.css"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, newRequest(http.MethodGet, path))
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s must not be served under a base path", path)
	}
}

func TestBasePath_RenderedHTMLPointsAtThePrefix(t *testing.T) {
	router := newBasePathRouter(t, "/team", false)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newRequest(http.MethodGet, "/team/"))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	for _, want := range []string{
		`href="/team/static/css/main.css`,
		`href="/team/static/css/themes.css`,
		`href="/team/api/config/custom.css"`,
		`src="/team/static/js/app.js`,
		`src="/team/api/config/custom.js"`,
		`hx-get="/team/api/ping?groupName=Apps&amp;serviceName=Plex"`,
		`<meta name="base-path" content="/team">`,
	} {
		assert.Contains(t, body, want)
	}

	// Not a single URL may still point at the host root: that is the failure
	// this whole phase exists to prevent, and it is invisible in a screenshot
	// until the asset 404s.
	for _, unwanted := range []string{`href="/static/`, `src="/static/`, `href="/api/`, `src="/api/`, `hx-get="/api/`} {
		assert.NotContains(t, body, unwanted)
	}
}

func TestBasePath_AbsentLeavesTheHTMLUntouched(t *testing.T) {
	router := newBasePathRouter(t, "", false)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newRequest(http.MethodGet, "/"))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	// The regression half of the feature: with no prefix configured the page
	// is what it was before it existed — including the absence of the meta tag
	// that would otherwise be a byte-level difference on every deployment.
	assert.NotContains(t, body, "base-path")
	for _, want := range []string{
		`href="/static/css/main.css`,
		`href="/api/config/custom.css"`,
		`src="/static/js/app.js`,
		`hx-get="/api/ping?groupName=Apps&amp;serviceName=Plex"`,
	} {
		assert.Contains(t, body, want)
	}
	assert.NotContains(t, body, "/team")
}

func TestBasePath_AuthChallengeRedirectsInsideThePrefix(t *testing.T) {
	router := newBasePathRouter(t, "/team", true)

	rec := httptest.NewRecorder()
	req := newRequest(http.MethodGet, "/team/api/services")
	req.Header.Set("Accept", "text/html")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	// `next` is dashboard-relative — the prefix is added back when the
	// destination is finally used, which is what keeps it inside.
	assert.Equal(t, "/team/auth/login?next=%2Fapi%2Fservices", rec.Header().Get("Location"))

	rec = httptest.NewRecorder()
	req = newRequest(http.MethodGet, "/team/api/services")
	req.Header.Set("HX-Request", "true")
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, "/team/auth/login", rec.Header().Get("HX-Redirect"))
}

func TestBasePath_LoginPageStartsTheFlowInsideThePrefix(t *testing.T) {
	router := newBasePathRouter(t, "/team", true)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newRequest(http.MethodGet, "/team/auth/login"))
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, `href="/team/auth/google/start"`)
	assert.Contains(t, body, `href="/team/static/css/main.css`)
}

func TestBasePath_LoginCSSStaysReachable(t *testing.T) {
	// /static/ is on the gate's public allowlist. Under a prefix the request
	// arrives as /team/static/…, and if the prefix were matched instead of
	// stripped it would no longer match — the login page would render
	// unstyled behind its own gate. This is that regression, pinned.
	router := newBasePathRouter(t, "/team", true)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newRequest(http.MethodGet, "/team/static/css/main.css"))
	assert.NotEqual(t, http.StatusFound, rec.Code, "the login page CSS must not be gated")
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, newRequest(http.MethodGet, "/team/api/healthcheck"))
	assert.Equal(t, http.StatusOK, rec.Code, "the compose healthcheck calls this without credentials")
}

func TestBasePath_SessionCookieIsScopedToTheDashboard(t *testing.T) {
	newBasePathRouter(t, "/team", true)
	prefixed := rootDashboard(t)
	cfg := prefixed.Auth().Config
	require.NotNil(t, cfg)

	rec := httptest.NewRecorder()
	require.NoError(t, issueTestSession(rec, prefixed, cfg))
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "/team", cookies[0].Path,
		"a cookie with Path=/ would be sent to every other dashboard on the host")

	rec = httptest.NewRecorder()
	unprefixed := config.NewDashboard("", "", prefixed.Dir)
	require.NoError(t, issueTestSession(rec, unprefixed, cfg))
	assert.Equal(t, "/", rec.Result().Cookies()[0].Path,
		"at the root the cookie path must stay exactly what it was")
}

func TestBasePath_OAuthStateCookieNameIsPerDashboard(t *testing.T) {
	// __Host- requires Path=/, so this cookie cannot be scoped by path. Two
	// prefixes on the same host are separated by the name instead; without
	// that, a login started under one prefix overwrites the other's state.
	router := newBasePathRouter(t, "/team", true)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newRequest(http.MethodGet, "/team/auth/google/start"))
	require.Equal(t, http.StatusFound, rec.Code)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "myserver_oauth_team", cookies[0].Name)
	assert.Equal(t, "/", cookies[0].Path, "__Host- semantics: this one stays at the root")
}

func TestOAuthCookieSuffix(t *testing.T) {
	cases := map[string]string{
		"":      "",
		"/team": "_team",
		"/a/b":  "_a_b",
	}
	for prefix, want := range cases {
		if got := oauthCookieSuffix(prefix); got != want {
			t.Errorf("oauthCookieSuffix(%q) = %q, want %q", prefix, got, want)
		}
	}
}

func TestSafeNext_StaysInsideTheDashboardOncePrefixed(t *testing.T) {
	// safeNext keeps a destination same-site and dashboard-relative; the
	// prefixing at emit time is what confines it to THIS dashboard. A stored
	// `next` naming another prefix cannot escape, because it is re-rooted.
	for _, next := range []string{"/other/secret", "/", "/api/services", "//evil.com", "/\\evil.com", ""} {
		got := config.PrefixPath("/team", safeNext(next))
		assert.True(t, strings.HasPrefix(got, "/team/"),
			"next=%q resolved to %q, which leaves the dashboard", next, got)
	}
}

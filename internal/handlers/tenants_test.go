package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thotenn/myserver/internal/auth"
	"github.com/thotenn/myserver/internal/config"
	"go.uber.org/zap"
)

// These are the tests the multi-dashboard work exists to make possible to
// write. One process serves several people's dashboards off one hostname, so
// "does dashboard A ever answer with dashboard B's data, or act with its
// credentials?" is not a property to review for — it is a property to pin.

const rootServices = `
- Mine:
    - Grafana:
        href: https://grafana.internal
        ping: grafana.internal
        siteMonitor: https://grafana.internal/health
        widget:
          type: customapi
          url: https://grafana.internal/api
          key: root-api-key
`

const acmeServices = `
- Acme:
    - Wiki:
        href: https://wiki.acme.example
        ping: wiki.acme.example
`

const globexServices = `
- Globex:
    - Intranet:
        href: https://intranet.globex.example
`

// tenantRouter publishes a root dashboard plus two clients and returns the
// real router, edge included.
func tenantRouter(t *testing.T, rootFiles map[string]string, clients map[string]map[string]string) http.Handler {
	t.Helper()
	if rootFiles == nil {
		rootFiles = map[string]string{"services.yaml": rootServices}
	}
	withDashboards(t, rootFiles, clients)
	return API(zap.NewNop(), 3000)
}

func defaultTenants() map[string]map[string]string {
	return map[string]map[string]string{
		"acme":   {"services.yaml": acmeServices},
		"globex": {"services.yaml": globexServices},
	}
}

func get(t *testing.T, router http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := newRequest(http.MethodGet, path)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// getFrom is get from a distinct client address, for the endpoints whose rate
// limit is tighter than the number of dashboards a test walks.
func getFrom(t *testing.T, router http.Handler, ip, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := newRequest(http.MethodGet, path)
	req.RemoteAddr = ip + ":54321"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// The leak the whole phase exists to prevent: a single process-wide merged
// cache used to answer whichever dashboard asked with whatever the previous
// request left behind.
func TestTenants_ServicesDoNotCross(t *testing.T) {
	router := tenantRouter(t, nil, defaultTenants())

	// Warm the root's cache first — that ordering is what used to poison the
	// clients' responses.
	root := get(t, router, "/api/services").Body.String()
	require.Contains(t, root, "Grafana")

	acme := get(t, router, "/acme/api/services").Body.String()
	assert.Contains(t, acme, "Wiki")
	assert.NotContains(t, acme, "Grafana", "a client was served the root dashboard's services")
	assert.NotContains(t, acme, "Intranet", "a client was served another client's services")

	globex := get(t, router, "/globex/api/services").Body.String()
	assert.Contains(t, globex, "Intranet")
	assert.NotContains(t, globex, "Wiki")
	assert.NotContains(t, globex, "Grafana")

	// And the root still answers with its own after the clients have run.
	assert.Contains(t, get(t, router, "/api/services").Body.String(), "Grafana")
}

func TestTenants_HashIsPerDashboard(t *testing.T) {
	router := tenantRouter(t, nil, defaultTenants())

	ip := 0
	hashOf := func(path string) string {
		ip++
		rec := getFrom(t, router, fmt.Sprintf("198.51.100.%d", ip), path)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var payload map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		assert.NotEmpty(t, payload["hash"])
		return payload["hash"]
	}

	// Distinct, or editing one client's YAML would reload every other
	// dashboard's open browser.
	assert.NotEqual(t, hashOf("/api/hash"), hashOf("/acme/api/hash"))
	assert.NotEqual(t, hashOf("/acme/api/hash"), hashOf("/globex/api/hash"))
}

// The widget proxy resolves a group and service name against a config file and
// then calls the upstream with the credentials it finds there. For a client
// dashboard the route does not exist at all — which is a stronger statement
// than "it checks".
func TestTenants_ProxyDoesNotExistForAClient(t *testing.T) {
	router := tenantRouter(t, nil, defaultTenants())

	rec := get(t, router, "/acme/api/services/proxy?group=Mine&service=Grafana&endpoint=default")
	assert.Equal(t, http.StatusNotFound, rec.Code,
		"the widget proxy must not exist for a client: it forwards credentials from a config file")
	assert.NotContains(t, rec.Body.String(), "root-api-key")
}

// Everything else that describes the host, runs on it, or mutates it.
func TestTenants_HostSurfaceIsAbsentForAClient(t *testing.T) {
	t.Setenv("HOMEPAGE_SCRIPTS_ENABLED", "true")
	router := tenantRouter(t, nil, defaultTenants())

	for _, path := range []string{
		"/acme/api/services/proxy?group=Acme&service=Wiki&endpoint=default",
		"/acme/api/docker/stats/container/server",
		"/acme/api/docker/status/container/server",
		"/acme/api/proxmox/stats/100/pve",
		"/acme/api/widgets/resources",
		"/acme/api/widgets/openmeteo",
		"/acme/api/validate",
		"/acme/api/scripts",
		"/acme/api/scripts/backup/status",
	} {
		rec := get(t, router, path)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s must not exist for a client dashboard", path)
	}

	// POST-only routes.
	for _, path := range []string{"/acme/api/reload", "/acme/api/scripts/backup"} {
		req := newRequest(http.MethodPost, path)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s must not exist for a client dashboard", path)
	}

	// The same routes are there for the root dashboard, so the assertions
	// above are about the client surface and not about a broken router.
	assert.NotEqual(t, http.StatusNotFound, get(t, router, "/api/validate").Code)
}

// ping and siteMonitor are the two outbound calls a client CAN cause, so both
// resolve against that client's own services and refuse anything else.
func TestTenants_ProbesCannotReachAnotherDashboardsHosts(t *testing.T) {
	router := tenantRouter(t, nil, defaultTenants())

	// By host/url: the target is named in the root's services.yaml.
	assert.Equal(t, http.StatusForbidden,
		get(t, router, "/acme/api/ping?host=grafana.internal").Code,
		"a client pinged a host it does not have in its own config")
	assert.Equal(t, http.StatusForbidden,
		get(t, router, "/acme/api/siteMonitor?url=https://grafana.internal/health").Code,
		"a client probed a URL it does not have in its own config")

	// By group/service name: the names exist, but in another dashboard.
	rec := get(t, router, "/acme/api/ping?groupName=Mine&serviceName=Grafana")
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"a group/service from another dashboard must not resolve")
	assert.NotContains(t, rec.Body.String(), "grafana.internal")

	// Its own service resolves, so the refusals above are about ownership.
	assert.Equal(t, http.StatusForbidden,
		get(t, router, "/globex/api/ping?host=wiki.acme.example").Code)
}

// custom.css / custom.js and any image are read from the dashboard's own
// directory. Reading them from a shared one would publish whatever the root
// operator dropped in there.
func TestTenants_ConfigFilesComeFromTheirOwnDirectory(t *testing.T) {
	router := tenantRouter(t,
		map[string]string{"services.yaml": rootServices, "custom.css": "body{--root:1}"},
		map[string]map[string]string{
			"acme":   {"services.yaml": acmeServices, "custom.css": "body{--acme:1}"},
			"globex": {"services.yaml": globexServices},
		})

	assert.Contains(t, get(t, router, "/api/config/custom.css").Body.String(), "--root")

	acme := get(t, router, "/acme/api/config/custom.css").Body.String()
	assert.Contains(t, acme, "--acme")
	assert.NotContains(t, acme, "--root")

	// A client without one gets an empty file, never the root's.
	assert.Empty(t, get(t, router, "/globex/api/config/custom.css").Body.String())
}

// A directory traversal out of a client's config dir must not reach the root's.
func TestTenants_ConfigFileTraversalStaysInTheDashboard(t *testing.T) {
	router := tenantRouter(t,
		map[string]string{"services.yaml": rootServices, "custom.css": "body{--root:1}"},
		defaultTenants())

	for _, p := range []string{"../../custom.css", "..%2F..%2Fcustom.css"} {
		rec := get(t, router, "/acme/api/config/"+p)
		assert.NotContains(t, rec.Body.String(), "--root",
			"traversal out of the client's config dir reached the root's: %s", p)
	}
}

const rootAuth = `
allowlist:
  emails: [operator@example.com]
google:
  clientId: "root-id"
  clientSecret: "root-secret"
  redirectURL: "https://dashboard.example.com/auth/google/callback"
session:
  secret: "root-session-secret"
  secure: false
`

const acmeAuth = `
allowlist:
  emails: [operator@example.com, buyer@acme.example]
google:
  clientId: "root-id"
  clientSecret: "root-secret"
  redirectURL: "https://dashboard.example.com/auth/google/callback"
session:
  secret: "acme-session-secret"
  secure: false
`

// acmeSharedSecretAuth is the copy-paste mistake: the same signing key as the
// root dashboard, so only the allowlist is left to separate them.
const acmeSharedSecretAuth = `
allowlist:
  emails: [buyer@acme.example]
google:
  clientId: "root-id"
  clientSecret: "root-secret"
  redirectURL: "https://dashboard.example.com/auth/google/callback"
session:
  secret: "root-session-secret"
  secure: false
`

func authedTenants(t *testing.T, clientAuth string) http.Handler {
	t.Helper()
	return tenantRouter(t,
		map[string]string{"services.yaml": rootServices, config.AuthFile: rootAuth},
		map[string]map[string]string{
			"acme": {"services.yaml": acmeServices, config.AuthFile: clientAuth},
		})
}

// mintSession issues a session cookie for a dashboard, the way its own login
// would.
func mintSession(t *testing.T, d *config.Dashboard, email string) *http.Cookie {
	t.Helper()
	cfg := d.Auth().Config
	require.NotNil(t, cfg, "dashboard %s has no auth policy", d)
	rec := httptest.NewRecorder()
	require.NoError(t, auth.IssueSession(rec, d, cfg, email))
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

func TestTenants_SessionCookieNamesAndPathsAreDistinct(t *testing.T) {
	authedTenants(t, acmeAuth)
	set := config.Dashboards()

	rootCookie := mintSession(t, set.Root(), "operator@example.com")
	acmeCookie := mintSession(t, set.Client("acme"), "buyer@acme.example")

	assert.Equal(t, "myserver_session", rootCookie.Name)
	assert.Equal(t, "myserver_session_acme", acmeCookie.Name,
		"same-named cookies on one host are ambiguous: the browser sends both to /acme")
	assert.Equal(t, "/", rootCookie.Path)
	assert.Equal(t, "/acme", acmeCookie.Path)
}

// The cookie a client holds must not authenticate anywhere else, even after
// the holder renames it to the other dashboard's cookie name — which is all it
// takes, since they share a hostname.
func TestTenants_SessionDoesNotAuthenticateAnotherDashboard(t *testing.T) {
	router := authedTenants(t, acmeAuth)
	set := config.Dashboards()

	acmeCookie := mintSession(t, set.Client("acme"), "buyer@acme.example")
	assert.Equal(t, http.StatusOK, get(t, router, "/acme/api/services", acmeCookie).Code,
		"precondition: the cookie works on its own dashboard")

	// Renamed to the root's cookie name and presented at the root.
	forged := &http.Cookie{Name: "myserver_session", Value: acmeCookie.Value}
	rec := get(t, router, "/api/services", forged)
	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"a client's session signed the caller into the root dashboard")
	assert.NotContains(t, rec.Body.String(), "Grafana")

	// And the other way round.
	rootCookie := mintSession(t, set.Root(), "operator@example.com")
	forged = &http.Cookie{Name: "myserver_session_acme", Value: rootCookie.Value}
	rec = get(t, router, "/acme/api/services", forged)
	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"the root's session signed the caller into a client dashboard")
	assert.NotContains(t, rec.Body.String(), "Wiki")
}

// Defence in depth: even with the signing keys accidentally identical, the
// allowlist is re-checked per request against the dashboard being served.
func TestTenants_SharedSigningKeyStillHonoursEachAllowlist(t *testing.T) {
	router := authedTenants(t, acmeSharedSecretAuth)
	set := config.Dashboards()

	acme := set.Client("acme")
	cookie := mintSession(t, acme, "buyer@acme.example")
	assert.Equal(t, http.StatusOK, get(t, router, "/acme/api/services", cookie).Code,
		"precondition: the cookie works on its own dashboard")

	// The signature verifies at the root too — the key is the same — so the
	// allowlist is the only thing left, and it has to be enough.
	forged := &http.Cookie{Name: "myserver_session", Value: cookie.Value}
	rec := get(t, router, "/api/services", forged)
	assert.Equal(t, http.StatusForbidden, rec.Code,
		"an address on one dashboard's allowlist is not on another's")
	assert.NotContains(t, rec.Body.String(), "Grafana")
}

// A public client dashboard next to a gated root must not become a way around
// the root's gate.
func TestTenants_APublicClientDoesNotOpenTheRoot(t *testing.T) {
	router := tenantRouter(t,
		map[string]string{"services.yaml": rootServices, config.AuthFile: rootAuth},
		map[string]map[string]string{"acme": {"services.yaml": acmeServices}})

	assert.Equal(t, http.StatusOK, get(t, router, "/acme/api/services").Code,
		"a client without an auth.yaml is public, which is allowed")

	rec := get(t, router, "/api/services")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.NotContains(t, rec.Body.String(), "Grafana")
}

// A client whose auth policy can no longer be determined fails closed on its
// own subtree and leaves every other dashboard serving.
func TestTenants_AClientInLockdownDoesNotTakeDownTheHost(t *testing.T) {
	router := tenantRouter(t,
		map[string]string{"services.yaml": rootServices},
		map[string]map[string]string{
			"acme":   {"services.yaml": acmeServices, config.AuthFile: acmeAuth},
			"globex": {"services.yaml": globexServices},
		})

	acme := config.Dashboards().Client("acme")
	require.True(t, acme.Auth().Required, "precondition")

	// The client's auth.yaml vanishes — a failed mount, or a bad deploy. That
	// is indistinguishable from "somebody deleted the gate", so it locks down
	// rather than reverting to public.
	require.NoError(t, os.Remove(filepath.Join(acme.Dir, config.AuthFile)))
	acme.Reload()
	require.True(t, acme.Auth().Lockdown, "precondition")

	rec := get(t, router, "/acme/api/services")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"an unreadable policy must refuse to serve, never fall back to public")
	assert.NotContains(t, rec.Body.String(), "Wiki")

	assert.Equal(t, http.StatusOK, get(t, router, "/api/services").Code)
	assert.Equal(t, http.StatusOK, get(t, router, "/globex/api/services").Code)
}

// A dashboard directory that appears while the process is running is served
// without a restart — the reason the alta of a client is one step.
func TestTenants_ADashboardAddedAtRuntimeIsServed(t *testing.T) {
	router := tenantRouter(t, nil, defaultTenants())
	assert.Equal(t, http.StatusNotFound, get(t, router, "/initech/api/services").Code)

	root := config.Dashboards().Root()
	writeFixtures(t, root.Dir+"/"+config.DashboardsSubdir+"/initech",
		map[string]string{"services.yaml": "- Initech:\n    - Portal:\n        href: https://portal.initech.example\n"})

	set, errs := config.InitDashboards()
	require.Empty(t, errs)
	set.Client("initech").Reload()

	body := get(t, router, "/initech/api/services").Body.String()
	assert.Contains(t, body, "Portal")

	// The dashboards that were already being served keep their identity, so
	// nobody is signed out and no cache is dropped by someone else's arrival.
	assert.Same(t, root, set.Root())
}

// A directory that would shadow the root router's own routes is refused rather
// than served, and the routes it would have shadowed keep working.
func TestTenants_ReservedSlugsAreRefused(t *testing.T) {
	dir := t.TempDir()
	for _, slug := range []string{"api", "auth", "static", "bad slug", "ok"} {
		writeFixtures(t, dir+"/"+config.DashboardsSubdir+"/"+slug, map[string]string{"services.yaml": acmeServices})
	}
	set, errs := config.ScanDashboards(nil, dir, "")

	assert.Len(t, errs, 4, "every reserved or malformed name should be reported: %v", errs)
	assert.NotNil(t, set.Client("ok"))
	for _, slug := range []string{"api", "auth", "static", "bad slug"} {
		assert.Nil(t, set.Client(slug), "%q must not become a dashboard", slug)
	}
}

// --- the shared OAuth callback -------------------------------------------
//
// One callback for every dashboard is what keeps the identity provider to a
// single registered redirect URI, and it works because the login carries the
// dashboard it belongs to in a signed payload. That slug picks the allowlist
// the login is judged against, so it is the security boundary of the feature.

// startLogin runs /auth/google/start for a dashboard and returns the state
// cookie it set.
func startLogin(t *testing.T, router http.Handler, prefix string) *http.Cookie {
	t.Helper()
	rec := get(t, router, prefix+"/auth/google/start")
	require.Equal(t, http.StatusFound, rec.Code, rec.Body.String())
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

func decodeState(t *testing.T, c *http.Cookie) oauthState {
	t.Helper()
	raw, ok := auth.OpenState(c.Value)
	require.True(t, ok, "the state cookie must carry a valid signature")
	var s oauthState
	require.NoError(t, json.Unmarshal(raw, &s))
	return s
}

func TestOAuth_StateCarriesTheDashboardItBelongsTo(t *testing.T) {
	router := authedTenants(t, acmeAuth)

	rootState := startLogin(t, router, "")
	assert.Equal(t, "myserver_oauth", rootState.Name)
	assert.Empty(t, decodeState(t, rootState).Dashboard, "the root dashboard's slug is empty")

	acmeState := startLogin(t, router, "/acme")
	assert.Equal(t, "myserver_oauth_acme", acmeState.Name,
		"two logins in flight on one host must not overwrite each other")
	assert.Equal(t, "/", acmeState.Path, "__Host- semantics: this cookie stays at the root")
	assert.Equal(t, "acme", decodeState(t, acmeState).Dashboard)
}

// The callback picks the flow it is completing out of every state cookie the
// browser holds, by matching the state the provider echoed back.
func TestOAuth_CallbackPicksTheFlowOutOfSeveralInFlight(t *testing.T) {
	router := authedTenants(t, acmeAuth)
	rootState := startLogin(t, router, "")
	acmeState := startLogin(t, router, "/acme")

	req := newRequest(http.MethodGet, "/auth/google/callback?state="+decodeState(t, acmeState).State)
	req.AddCookie(rootState)
	req.AddCookie(acmeState)

	found, name, inFlight := matchOAuthStateCookie(req)
	require.NotNil(t, found)
	assert.Equal(t, "acme", found.Dashboard, "the callback completed the wrong dashboard's login")
	assert.Equal(t, acmeState.Name, name)
	assert.Equal(t, 2, inFlight)
}

// Rewriting the slug is the attack the signature exists to stop: the cookie is
// in the caller's own browser, so nothing else would keep them from choosing
// which allowlist judges them.
func TestOAuth_ATamperedSlugIsRejected(t *testing.T) {
	router := authedTenants(t, acmeAuth)
	acmeState := startLogin(t, router, "/acme")
	stored := decodeState(t, acmeState)

	forgedPayload, err := json.Marshal(oauthState{
		State: stored.State, Nonce: stored.Nonce, Next: "/", Dashboard: "",
	})
	require.NoError(t, err)

	// The payload swapped, the original signature kept.
	sig := acmeState.Value[strings.LastIndex(acmeState.Value, ".")+1:]
	forged := base64.RawURLEncoding.EncodeToString(forgedPayload) + "." + sig

	req := newRequest(http.MethodGet, "/auth/google/callback?state="+stored.State)
	req.AddCookie(&http.Cookie{Name: acmeState.Name, Value: forged})

	found, _, inFlight := matchOAuthStateCookie(req)
	assert.Nil(t, found, "a re-signed-by-nobody payload was accepted")
	assert.Zero(t, inFlight)

	// And through the router: no session is issued, whatever else happens.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		assert.NotContains(t, c.Name, "myserver_session",
			"a tampered state cookie produced a session")
	}
}

// A login for a dashboard that was removed while it was in flight must not
// fall back to some other dashboard's policy.
func TestOAuth_CallbackForARemovedDashboardIssuesNothing(t *testing.T) {
	router := authedTenants(t, acmeAuth)
	acmeState := startLogin(t, router, "/acme")

	// The client is decommissioned while the browser is at Google.
	require.NoError(t, os.RemoveAll(filepath.Join(config.Dashboards().Root().Dir,
		config.DashboardsSubdir, "acme")))
	set, errs := config.InitDashboards()
	require.Empty(t, errs)
	require.Nil(t, set.Client("acme"), "precondition: the dashboard is gone")

	req := newRequest(http.MethodGet, "/auth/google/callback?state="+decodeState(t, acmeState).State)
	req.AddCookie(acmeState)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		assert.NotContains(t, c.Name, "myserver_session",
			"a login for a dashboard that no longer exists issued a session")
	}
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/thotenn/myserver/internal/config"
)

// echo reports what the wrapped handler actually sees, which is the whole
// point of the edge: everything downstream must see today's paths, and must be
// able to say which dashboard it is serving.
func echo(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Router", name)
		w.Header().Set("X-Seen-Path", r.URL.Path)
		w.Header().Set("X-Seen-RequestURI", r.URL.RequestURI())
		w.Header().Set("X-Seen-BasePath", config.BasePathFrom(r.Context()))
		if d := config.DashboardFrom(r.Context()); d != nil {
			w.Header().Set("X-Seen-Dashboard", d.String())
		}
		w.WriteHeader(http.StatusOK)
	})
}

// dispatcher publishes a registry built from a temp tree and returns the edge
// over two distinguishable routers.
func dispatcher(t *testing.T, basePath string, slugs ...string) http.Handler {
	t.Helper()
	root := t.TempDir()
	for _, slug := range slugs {
		if err := os.MkdirAll(filepath.Join(root, config.DashboardsSubdir, slug), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	set, errs := config.ScanDashboards(nil, root, basePath)
	if len(errs) > 0 {
		t.Fatalf("scanning dashboards: %v", errs)
	}
	config.SetDashboards(set)
	t.Cleanup(func() { config.SetDashboards(nil) })
	return Dispatch(echo("root"), echo("client"))
}

func TestDispatch_BasePathIsStrippedAndPublished(t *testing.T) {
	h := dispatcher(t, "/team")

	cases := map[string]string{
		"/team":                  "/",
		"/team/":                 "/",
		"/team/api/services":     "/api/services",
		"/team/static/css/a.css": "/static/css/a.css",
	}
	for request, seen := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, request, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", request, rec.Code)
			continue
		}
		if got := rec.Header().Get("X-Seen-Path"); got != seen {
			t.Errorf("%s: handler saw %q, want %q", request, got, seen)
		}
		if got := rec.Header().Get("X-Seen-BasePath"); got != "/team" {
			t.Errorf("%s: context base path = %q, want %q", request, got, "/team")
		}
		if got := rec.Header().Get("X-Router"); got != "root" {
			t.Errorf("%s: reached the %s router", request, got)
		}
	}
}

func TestDispatch_QueryAndEscapesSurvive(t *testing.T) {
	h := dispatcher(t, "/team")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/team/api/docker/stats/my%20box/local?x=1", nil))
	if got, want := rec.Header().Get("X-Seen-Path"), "/api/docker/stats/my box/local"; got != want {
		t.Errorf("decoded path = %q, want %q", got, want)
	}
	// RequestURI keeps the escaped form, which is what `next` is built from.
	if got, want := rec.Header().Get("X-Seen-RequestURI"), "/api/docker/stats/my%20box/local?x=1"; got != want {
		t.Errorf("RequestURI = %q, want %q", got, want)
	}
}

func TestDispatch_OutsideThePrefixIs404(t *testing.T) {
	h := dispatcher(t, "/team")

	for _, path := range []string{"/", "/api/services", "/teamwork", "/teamwork/api", "/other/team"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
	}
}

// The single-dashboard deployment: nothing is stripped, nothing is rewritten,
// and the router downstream is handed the URL that arrived.
func TestDispatch_NoPrefixLeavesTheRequestAlone(t *testing.T) {
	h := dispatcher(t, "")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services?a=b", nil))
	if got, want := rec.Header().Get("X-Seen-Path"), "/api/services"; got != want {
		t.Errorf("handler saw %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("X-Seen-RequestURI"), "/api/services?a=b"; got != want {
		t.Errorf("RequestURI = %q, want %q", got, want)
	}
	if got := rec.Header().Get("X-Seen-BasePath"); got != "" {
		t.Errorf("context base path = %q, want empty", got)
	}
	if got, want := rec.Header().Get("X-Seen-Dashboard"), "root"; got != want {
		t.Errorf("dashboard = %q, want %q", got, want)
	}
}

func TestDispatch_ClientSlugReachesTheClientRouter(t *testing.T) {
	h := dispatcher(t, "", "acme")

	cases := map[string]string{
		"/acme":              "/",
		"/acme/":             "/",
		"/acme/api/services": "/api/services",
	}
	for request, seen := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, request, nil))
		if got := rec.Header().Get("X-Router"); got != "client" {
			t.Errorf("%s: reached the %s router, want client", request, got)
		}
		if got := rec.Header().Get("X-Seen-Path"); got != seen {
			t.Errorf("%s: handler saw %q, want %q", request, got, seen)
		}
		if got, want := rec.Header().Get("X-Seen-Dashboard"), "acme"; got != want {
			t.Errorf("%s: dashboard = %q, want %q", request, got, want)
		}
		if got, want := rec.Header().Get("X-Seen-BasePath"), "/acme"; got != want {
			t.Errorf("%s: base path = %q, want %q", request, got, want)
		}
	}
}

// A client dashboard lives under the process base path too, so its own prefix
// is the concatenation of the two.
func TestDispatch_ClientUnderABasePath(t *testing.T) {
	h := dispatcher(t, "/team", "acme")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/team/acme/api/hash", nil))
	if got, want := rec.Header().Get("X-Seen-Path"), "/api/hash"; got != want {
		t.Errorf("handler saw %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("X-Seen-BasePath"), "/team/acme"; got != want {
		t.Errorf("base path = %q, want %q", got, want)
	}

	// The root dashboard still answers on the bare base path.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/team/api/hash", nil))
	if got := rec.Header().Get("X-Router"); got != "root" {
		t.Errorf("the root dashboard was routed to the %s router", got)
	}
}

// An unknown first segment is NOT a dashboard: it belongs to the root router,
// which is what answers 404 for it. Treating it as a missing dashboard here
// would turn every mistyped root URL into a different error.
func TestDispatch_UnknownSegmentStaysOnTheRootRouter(t *testing.T) {
	h := dispatcher(t, "", "acme")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/acmeworks/api/services", nil))
	if got := rec.Header().Get("X-Router"); got != "root" {
		t.Errorf("reached the %s router, want root", got)
	}
	if got, want := rec.Header().Get("X-Seen-Path"), "/acmeworks/api/services"; got != want {
		t.Errorf("handler saw %q, want %q", got, want)
	}
}

// Without a registry there is no way to say who owns a path, and answering
// from "the" config is the failure the whole design removes.
func TestDispatch_WithoutARegistryRefuses(t *testing.T) {
	config.SetDashboards(nil)
	h := Dispatch(echo("root"), echo("client"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

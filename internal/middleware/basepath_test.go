package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thotenn/myserver/internal/config"
)

// echoPath reports what the wrapped handler actually sees, which is the whole
// point of the middleware: everything downstream must see today's paths.
func echoPath() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Path", r.URL.Path)
		w.Header().Set("X-Seen-RequestURI", r.URL.RequestURI())
		w.Header().Set("X-Seen-BasePath", config.BasePathFrom(r.Context()))
		w.WriteHeader(http.StatusOK)
	})
}

func TestBasePath_StripsThePrefixAndPublishesItOnTheContext(t *testing.T) {
	h := BasePath("/team")(echoPath())

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
	}
}

func TestBasePath_QueryAndEscapesSurvive(t *testing.T) {
	h := BasePath("/team")(echoPath())

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

func TestBasePath_OutsideThePrefixIs404(t *testing.T) {
	h := BasePath("/team")(echoPath())

	for _, path := range []string{"/", "/api/services", "/teamwork", "/teamwork/api", "/other/team"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, rec.Code)
		}
	}
}

// countingHandler is a pointer type so the identity check below is a real
// pointer comparison rather than a comparison of function values.
type countingHandler struct{ calls int }

func (h *countingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.calls++
	w.WriteHeader(http.StatusOK)
}

func TestBasePath_EmptyPrefixReturnsTheHandlerUntouched(t *testing.T) {
	inner := &countingHandler{}
	if got := BasePath("")(inner); got != http.Handler(inner) {
		t.Error("with no prefix the middleware must not wrap anything: an unprefixed " +
			"deployment has to run the same code it ran before this existed")
	}
}

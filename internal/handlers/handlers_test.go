package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thotenn/myserver/internal/config"
	"go.uber.org/zap"
)

// withTempConfig sets up an isolated config dir with the supplied YAML
// fixtures and returns a cleanup function.
func withTempConfig(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	prev := os.Getenv("HOMEPAGE_CONFIG_DIR")
	t.Setenv("HOMEPAGE_CONFIG_DIR", dir)
	config.SetConfigDir(dir)
	t.Cleanup(func() {
		config.ResetConfigDir()
		if prev != "" {
			_ = os.Setenv("HOMEPAGE_CONFIG_DIR", prev)
		}
	})
	return dir
}

func TestServices_StripsBasicAuthAndCredentials(t *testing.T) {
	withTempConfig(t, map[string]string{
		"services.yaml": `
- Apps:
    - Plex:
        href: https://plex.example.com
        widget:
          type: plex
          url: https://admin:hunter2@plex.example.com/api?token=secret123
          key: super-secret
          mappings:
            - field: a
              label: A
`,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	rec := httptest.NewRecorder()
	Services(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "hunter2", "basic-auth password leaked")
	assert.NotContains(t, body, "secret123", "token query param leaked")
	assert.NotContains(t, body, "super-secret", "widget.Key leaked")
	// Mappings should be preserved.
	assert.Contains(t, body, "mappings")
}

func TestWidgets_SanitizesNestedCredentials(t *testing.T) {
	withTempConfig(t, map[string]string{
		"widgets.yaml": `
- openweathermap:
    label: Home
    apiKey: should-be-redacted
    units: metric
- search:
    provider: google
- customapi:
    url: https://api.example.com
    headers:
      Authorization: Bearer xxx
    body:
      clientSecret: leaked-secret
      visible: keep-me
`,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/widgets", nil)
	rec := httptest.NewRecorder()
	Widgets(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "should-be-redacted")
	assert.NotContains(t, body, "Bearer xxx")
	assert.NotContains(t, body, "leaked-secret")
	assert.Contains(t, body, "keep-me", "non-sensitive nested field should be preserved")
	assert.Contains(t, body, "metric")
}

func TestHash_ReflectsCurrentHash(t *testing.T) {
	withTempConfig(t, map[string]string{
		"settings.yaml": `title: foo`,
	})
	// Seed CurrentHash via ConfigHash
	h, err := config.ConfigHash()
	require.NoError(t, err)
	config.SetCurrentHash(h)

	req := httptest.NewRequest(http.MethodGet, "/api/hash", nil)
	rec := httptest.NewRecorder()
	Hash(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Equal(t, h, payload["hash"])
}

func TestRunScript_DisabledReturns404(t *testing.T) {
	t.Setenv("HOMEPAGE_SCRIPTS_ENABLED", "false")
	ScriptManager = nil

	req := httptest.NewRequest(http.MethodPost, "/api/scripts/whatever", nil)
	rec := httptest.NewRecorder()
	RunScript(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestConfigFile_PathTraversalRejected(t *testing.T) {
	// path is bound to {path} chi var; we use a chi router to populate it.
	for _, p := range []string{"secrets.yaml", "../etc/passwd", "settings.yaml"} {
		req := httptest.NewRequest(http.MethodGet, "/api/config/"+p, nil)
		rec := httptest.NewRecorder()
		// Manually invoke the handler with chi context
		router := newTestRouter()
		router.ServeHTTP(rec, req)
		// All non-whitelisted paths should fail with 404, not 200 with body.
		if p != "custom.css" && p != "custom.js" {
			assert.NotEqual(t, http.StatusOK, rec.Code, "path %s should be rejected", p)
		}
	}
}

// newTestRouter wires the API handler enough to exercise the
// /api/config/{path} route.
func newTestRouter() http.Handler {
	return API(zap.NewNop(), 3000)
}

// TestServices_ConcurrentRequestsDoNotMutateCache is a regression test for a
// data race: the handler used to sanitize the merged service list in place,
// writing to the very slice stored in mergedServicesCache while concurrent
// requests were serializing it. Run with -race to see the original failure.
//
// It also pins the observable contract the in-place version happened to get
// right by luck — every response must be identical and fully sanitized, no
// matter how many requests raced or which one populated the cache.
func TestServices_ConcurrentRequestsDoNotMutateCache(t *testing.T) {
	withTempConfig(t, map[string]string{
		"services.yaml": `
- Apps:
    - Grafana:
        href: https://user:pass@grafana.example.com
        widget:
          type: customapi
          url: https://user:pass@grafana.example.com/api
          key: super-secret-key
          headers:
            Authorization: Bearer nope
    - Uptime:
        href: https://uptime.example.com
        widget:
          type: customapi
          url: https://uptime.example.com/api
          apiKey: another-secret
`,
	})
	config.ReloadCache()

	const workers = 16
	bodies := make([]string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			Services(rec, httptest.NewRequest(http.MethodGet, "/api/services", nil))
			bodies[i] = rec.Body.String()
		}(i)
	}
	wg.Wait()

	for i, body := range bodies {
		assert.Equal(t, bodies[0], body, "response %d differs — the shared slice was mutated mid-flight", i)
		assert.NotContains(t, body, "super-secret-key")
		assert.NotContains(t, body, "another-secret")
		assert.NotContains(t, body, "Bearer nope")
		assert.NotContains(t, body, "user:pass")
	}

	// The cached value must itself be sanitized and must survive being
	// served repeatedly without being edited.
	cached, ok := mergedServicesCache.Load().([]config.ServiceGroup)
	require.True(t, ok, "cache not populated")
	require.NotEmpty(t, cached)
	for _, g := range cached {
		for _, s := range g.Services {
			require.NotNil(t, s.Widget)
			assert.Empty(t, s.Widget.Key)
			assert.Empty(t, s.Widget.APIKey)
			assert.Nil(t, s.Widget.Headers)
			assert.NotContains(t, s.Widget.URL, "user:pass")
		}
	}
}

// TestSanitizeServiceGroups_DoesNotTouchInput pins the copy semantics
// directly: the input must come back untouched, so the config cache and the
// merged list keep their credentials for the widget proxy to use.
func TestSanitizeServiceGroups_DoesNotTouchInput(t *testing.T) {
	in := []config.ServiceGroup{{
		Name: "Apps",
		Services: []config.Service{{
			Name: "Grafana",
			Widget: &config.WidgetConfig{
				Type:    "customapi",
				URL:     "https://user:pass@grafana.example.com/api",
				Key:     "keep-me",
				Headers: map[string]string{"Authorization": "Bearer keep-me"},
			},
		}},
	}}

	out := sanitizeServiceGroups(in)

	// Output sanitized…
	assert.Empty(t, out[0].Services[0].Widget.Key)
	assert.Nil(t, out[0].Services[0].Widget.Headers)
	assert.NotContains(t, out[0].Services[0].Widget.URL, "user:pass")
	// …input intact. The proxy reads credentials from this very struct.
	assert.Equal(t, "keep-me", in[0].Services[0].Widget.Key)
	assert.NotNil(t, in[0].Services[0].Widget.Headers)
	assert.Contains(t, in[0].Services[0].Widget.URL, "user:pass")
	// Backing arrays must not be shared.
	assert.NotSame(t, &in[0].Services[0], &out[0].Services[0])
	assert.NotSame(t, in[0].Services[0].Widget, out[0].Services[0].Widget)

	// nil in, nil out — the endpoint has always encoded that as JSON null.
	assert.Nil(t, sanitizeServiceGroups(nil))
}

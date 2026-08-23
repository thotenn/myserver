package templates

import (
	"context"
	"testing"

	"github.com/thotenn/myserver/internal/config"
)

// urlCases lists every builder against the same input twice: with no base path
// it must return the exact string it returned before the feature existed
// (that is the regression half), and with one it must be prefixed.
func urlCases(ctx context.Context) map[string]string {
	return map[string]string{
		"static":       staticURL(ctx, "/static/css/main.css", "abc123"),
		"staticNoVer":  staticURL(ctx, "/static/css/main.css", ""),
		"configFile":   configFileURL(ctx, "custom.css"),
		"auth":         authURL(ctx, "/auth/login"),
		"ping":         pingURL(ctx, "My Apps", "Plex & Co"),
		"siteMonitor":  siteMonitorURL(ctx, "My Apps", "Plex & Co"),
		"proxy":        proxyURL(ctx, "My Apps", "Plex & Co", ""),
		"dockerStats":  dockerStatsURL(ctx, "my container", "local"),
		"dockerStatus": dockerStatusURL(ctx, "my container", "local"),
		"script":       scriptRunURL(ctx, "backup db"),
		"resources":    resourcesURL(ctx, map[string]interface{}{"cpu": true}),
		"weather":      weatherURL(ctx, map[string]interface{}{"units": "metric"}),
	}
}

func TestURLBuilders_WithoutBasePathAreUnchanged(t *testing.T) {
	want := map[string]string{
		"static":       "/static/css/main.css?v=abc123",
		"staticNoVer":  "/static/css/main.css",
		"configFile":   "/api/config/custom.css",
		"auth":         "/auth/login",
		"ping":         "/api/ping?groupName=My+Apps&serviceName=Plex+%26+Co",
		"siteMonitor":  "/api/siteMonitor?groupName=My+Apps&serviceName=Plex+%26+Co",
		"proxy":        "/api/services/proxy?group=My+Apps&service=Plex+%26+Co&endpoint=default",
		"dockerStats":  "/api/docker/stats/my%20container/local",
		"dockerStatus": "/api/docker/status/my%20container/local",
		"script":       "/api/scripts/backup%20db",
		"resources":    "/api/widgets/resources?cpu=true",
		"weather":      "/api/widgets/openmeteo?units=metric",
	}
	got := urlCases(context.Background())
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s = %q, want %q", name, got[name], w)
		}
	}
}

func TestURLBuilders_CarryTheBasePath(t *testing.T) {
	ctx := config.WithBasePath(context.Background(), "/team")
	base := urlCases(context.Background())
	for name, prefixedURL := range urlCases(ctx) {
		want := "/team" + base[name]
		if prefixedURL != want {
			t.Errorf("%s = %q, want %q", name, prefixedURL, want)
		}
	}
}

func TestBackgroundImageURL_CarriesTheBasePath(t *testing.T) {
	root := backgroundImageURL(context.Background(), "wallpaper.jpg", "hash1")
	if root != "/api/config/wallpaper.jpg?v=hash1" {
		t.Errorf("without a base path = %q", root)
	}
	ctx := config.WithBasePath(context.Background(), "/team")
	if got := backgroundImageURL(ctx, "wallpaper.jpg", "hash1"); got != "/team"+root {
		t.Errorf("with a base path = %q, want %q", got, "/team"+root)
	}
	// An absolute URL is still returned verbatim: a prefix is a path, and
	// prefixing someone else's host would break the background entirely.
	if got := backgroundImageURL(ctx, "https://cdn.example.com/a.jpg", "h"); got != "https://cdn.example.com/a.jpg" {
		t.Errorf("absolute URL = %q, want it untouched", got)
	}
}

func TestBasePathOf(t *testing.T) {
	if got := basePathOf(context.Background()); got != "" {
		t.Errorf("basePathOf = %q, want empty so the meta tag is omitted", got)
	}
	ctx := config.WithBasePath(context.Background(), "/team")
	if got := basePathOf(ctx); got != "/team" {
		t.Errorf("basePathOf = %q, want %q", got, "/team")
	}
}

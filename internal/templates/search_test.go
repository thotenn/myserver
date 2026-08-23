package templates

import (
	"testing"

	"github.com/thotenn/myserver/internal/config"
)

func widgets(opts map[string]interface{}) []config.InfoWidget {
	return []config.InfoWidget{
		{Type: "datetime", Options: map[string]interface{}{}},
		{Type: "search", Options: opts},
	}
}

func TestSearchTarget_Providers(t *testing.T) {
	cases := []struct {
		name        string
		widgets     []config.InfoWidget
		action, prm string
		target      string
	}{
		// No search widget at all: the bar is not rendered, but the helper
		// must still answer with something submittable.
		{"none", nil, "https://www.google.com/search", "q", "_blank"},
		{"default", widgets(map[string]interface{}{}), "https://www.google.com/search", "q", "_blank"},
		{"duckduckgo", widgets(map[string]interface{}{"provider": "duckduckgo"}), "https://duckduckgo.com/", "q", "_blank"},
		{"case-insensitive", widgets(map[string]interface{}{"provider": "DuckDuckGo"}), "https://duckduckgo.com/", "q", "_blank"},
		// A provider with a different query parameter proves the name is not
		// assumed to be "q" anywhere.
		{"baidu", widgets(map[string]interface{}{"provider": "baidu"}), "https://www.baidu.com/s", "wd", "_blank"},
		{"startpage", widgets(map[string]interface{}{"provider": "startpage"}), "https://www.startpage.com/sp/search", "query", "_blank"},
		// An unknown engine falls back instead of rendering a form that goes
		// nowhere, which is the bug this replaced (action="/search").
		{"unknown", widgets(map[string]interface{}{"provider": "nope"}), "https://www.google.com/search", "q", "_blank"},
		{"target self", widgets(map[string]interface{}{"target": "_self"}), "https://www.google.com/search", "q", ""},
		// A custom self-hosted engine, Homepage's `url` semantics.
		{"custom url", widgets(map[string]interface{}{"url": "https://searx.example.com/search?q="}), "https://searx.example.com/search", "q", "_blank"},
		{"custom url other param", widgets(map[string]interface{}{"url": "https://s.example.com/find?query="}), "https://s.example.com/find", "query", "_blank"},
		{"custom url no param", widgets(map[string]interface{}{"url": "https://s.example.com/find"}), "https://s.example.com/find", "q", "_blank"},
		// Malformed or non-http values must not become the form action.
		{"custom url junk", widgets(map[string]interface{}{"url": "javascript:alert(1)"}), "https://www.google.com/search", "q", "_blank"},
		{"custom url relative", widgets(map[string]interface{}{"url": "/search?q="}), "https://www.google.com/search", "q", "_blank"},
		{"custom url two params", widgets(map[string]interface{}{"url": "https://s.example.com/?a=1&q="}), "https://www.google.com/search", "q", "_blank"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := searchTarget(c.widgets)
			if got.Action != c.action || got.Param != c.prm || got.Target != c.target {
				t.Errorf("searchTarget = %+v, want action=%q param=%q target=%q",
					got, c.action, c.prm, c.target)
			}
		})
	}
}

func TestSearchTarget_ActionIsAlwaysAbsolute(t *testing.T) {
	// The whole point: the form must post to a real engine. A relative action
	// means the dashboard is asked for a route it does not serve.
	for name := range searchProviders {
		got := searchTarget(widgets(map[string]interface{}{"provider": name}))
		if len(got.Action) < 8 || got.Action[:8] != "https://" {
			t.Errorf("provider %q resolved to %q, which is not an absolute https URL", name, got.Action)
		}
		if got.Param == "" {
			t.Errorf("provider %q has no query parameter", name)
		}
	}
}

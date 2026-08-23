package templates

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/thotenn/myserver/internal/config"
)

// The search bar is a progressively-enhanced form: `app.js` intercepts the
// submit to offer the live service/bookmark dropdown, but the form itself has
// to work on its own. It used to declare `action="/search"`, a route that does
// not exist — with JavaScript off, or before app.js ran, submitting the bar
// asked the dashboard for a page it does not serve.
//
// Now the action IS the configured search engine, which also makes
// `widgets.yaml: search.provider` mean something: the provider was previously
// hardcoded to Google inside app.js and the YAML value was ignored.

// searchProviders maps a `provider` value to its query URL and query
// parameter. Homepage's own set, minus the ones that need an API key.
var searchProviders = map[string]searchProvider{
	"google":     {"https://www.google.com/search", "q"},
	"duckduckgo": {"https://duckduckgo.com/", "q"},
	"bing":       {"https://www.bing.com/search", "q"},
	"brave":      {"https://search.brave.com/search", "q"},
	"startpage":  {"https://www.startpage.com/sp/search", "query"},
	"ecosia":     {"https://www.ecosia.org/search", "q"},
	"baidu":      {"https://www.baidu.com/s", "wd"},
	"qwant":      {"https://www.qwant.com/", "q"},
}

type searchProvider struct {
	URL   string
	Param string
}

// SearchTarget is what the search form needs to submit on its own.
type SearchTarget struct {
	Action string
	Param  string
	// Target is the anchor/form target: "_blank" opens the results in a new
	// tab, "" navigates in place.
	Target string
}

// defaultSearchProvider is Google, matching the behaviour before the provider
// was read from config at all.
const defaultSearchProvider = "google"

// searchTarget resolves the search widget's configuration into a form action.
//
//   - search: { provider: duckduckgo }        -> that engine
//   - search: { url: "https://s.example.com/?q=" } -> a custom engine, the
//     query is appended to the URL (Homepage's `url` semantics), so the form
//     posts to the part before the parameter.
//
// An unknown provider falls back to the default rather than rendering a form
// that goes nowhere.
func searchTarget(widgets []config.InfoWidget) SearchTarget {
	opts := map[string]interface{}{}
	for _, w := range widgets {
		if w.Type == "search" {
			opts = w.Options
			break
		}
	}

	target := "_blank"
	if v, ok := opts["target"]; ok {
		if s := fmt.Sprintf("%v", v); s == "_self" || s == "" {
			target = ""
		} else {
			target = s
		}
	}

	// A custom `url` wins: it is how Homepage points the bar at a self-hosted
	// engine (SearxNG, Whoogle...). It carries its own parameter name, e.g.
	// "https://searx.example.com/search?q=".
	if raw, ok := opts["url"]; ok {
		if action, param, ok := splitSearchURL(fmt.Sprintf("%v", raw)); ok {
			return SearchTarget{Action: action, Param: param, Target: target}
		}
	}

	name := defaultSearchProvider
	if v, ok := opts["provider"]; ok {
		if s := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v))); s != "" {
			name = s
		}
	}
	p, ok := searchProviders[name]
	if !ok {
		p = searchProviders[defaultSearchProvider]
	}
	return SearchTarget{Action: p.URL, Param: p.Param, Target: target}
}

// splitSearchURL turns "https://host/search?q=" into ("https://host/search",
// "q"). It only accepts absolute http(s) URLs whose query names exactly one
// empty parameter, so a malformed value falls back to the provider list
// instead of rendering a form that submits somewhere unintended.
func splitSearchURL(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", false
	}
	q := u.RawQuery
	if q == "" {
		// No parameter given: assume the conventional one.
		u.RawQuery = ""
		return u.String(), "q", true
	}
	param := strings.TrimSuffix(q, "=")
	if strings.ContainsAny(param, "=&") || param == "" {
		return "", "", false
	}
	u.RawQuery = ""
	return u.String(), param, true
}

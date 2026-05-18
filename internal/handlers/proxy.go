package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
	proxyhandlers "github.com/thotenn/myserver/internal/proxy/handlers"
	"github.com/thotenn/myserver/internal/templates"
	"github.com/thotenn/myserver/internal/widgets"
)

// proxyCache caches upstream widget responses to avoid hitting the backend
// on every HTMX poll. TTL defaults to 60s and the cache is invalidated on
// config reload.
var proxyCache = proxy.NewCache(60 * time.Second)

// InvalidateProxyCache clears all cached proxy responses. Called by the
// config watcher whenever configuration changes.
func InvalidateProxyCache() {
	if proxyCache != nil {
		proxyCache.DeleteAll()
	}
}

// proxyCacheKey returns a stable cache key for a widget + endpoint.
func proxyCacheKey(widget *config.WidgetConfig, endpoint string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(widget.Type))
	_, _ = h.Write([]byte(endpoint))
	_, _ = h.Write([]byte(widget.URL))
	return hex.EncodeToString(h.Sum(nil))
}

// MaxProxyBodyBytes caps the size of the body the proxy will forward
// upstream. Anything larger is rejected with 413.
const MaxProxyBodyBytes = 1 << 20 // 1 MiB

// Proxy handles /api/services/proxy requests. It looks up the service in
// services.yaml, validates the endpoint, and forwards the request via the
// generic proxy handler. Body size is capped to MaxProxyBodyBytes.
func Proxy(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	service := r.URL.Query().Get("service")
	endpoint := r.URL.Query().Get("endpoint")

	if group == "" || service == "" {
		http.Error(w, "missing group or service parameter", http.StatusBadRequest)
		return
	}

	widget := findServiceWidget(group, service)
	if widget == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	var body io.Reader
	if r.Method == http.MethodPost {
		// Cap incoming body to defeat DoS via large payloads.
		r.Body = http.MaxBytesReader(w, r.Body, MaxProxyBodyBytes)
		body = r.Body
	}

	// Check proxy cache first (GET only; POSTs are always forwarded).
	cacheKey := proxyCacheKey(widget, endpoint)
	var data interface{}
	if r.Method == http.MethodGet {
		if cached, ok := proxyCache.Get(cacheKey); ok {
			_ = json.Unmarshal(cached, &data)
		}
	}

	if data == nil {
		var err error
		data, err = proxyhandlers.GenericProxyHandler(r.Context(), widget, endpoint, body)
		if err != nil {
			writeProxyError(w, err)
			return
		}
		// Store successful response in cache.
		if r.Method == http.MethodGet {
			if bytes, err := json.Marshal(data); err == nil {
				proxyCache.Set(cacheKey, bytes, 60*time.Second)
			}
		}
	}

	// For customapi widgets with non-default display modes, apply display
	// processing (e.g. graph extraction).
	if widget.Type == "customapi" && widget.Display != "" && !strings.EqualFold(widget.Display, "dynamic-list") {
		var err error
		data, err = widgets.ProcessDisplay(data, widget.Display, widget.Mappings)
		if err != nil {
			writeProxyError(w, err)
			return
		}
	}

	// Content negotiation: when HTMX asks for a widget that opts into a
	// `display: dynamic-list` render, return server-side HTML instead of
	// the raw JSON payload. The service_card widget container uses
	// hx-swap="innerHTML" and the app.js beforeSwap guard rejects non-HTML
	// responses — so without this branch the list never renders.
	if isHTMXRequest(r) && strings.EqualFold(widget.Display, "dynamic-list") {
		items := extractDynamicListItems(data, widget.Mappings)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.DynamicListHTML(items).Render(r.Context(), w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// extractDynamicListItems walks a parsed JSON payload using the `mappings`
// block from a customapi widget and returns a flat slice of rows. The
// expected mappings shape is:
//
//	mappings:
//	  items: <top-level key pointing to an array>
//	  name:  <per-item field to use as visible text>
//	  label: <per-item field to use as right-side badge>
//	  target: "<template with {field} placeholders from the item>"
//
// Any missing or unexpectedly-typed field is silently tolerated — the
// widget degrades to an empty list rather than 500'ing the endpoint.
func extractDynamicListItems(data, mappings interface{}) []templates.DynamicListItem {
	m, ok := mappings.(map[string]interface{})
	if !ok {
		return nil
	}
	itemsKey, _ := m["items"].(string)
	nameKey, _ := m["name"].(string)
	labelKey, _ := m["label"].(string)
	targetTpl, _ := m["target"].(string)

	var rawItems []interface{}
	if itemsKey == "" {
		rawItems, _ = data.([]interface{})
	} else if obj, ok := data.(map[string]interface{}); ok {
		rawItems, _ = obj[itemsKey].([]interface{})
	}

	result := make([]templates.DynamicListItem, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, templates.DynamicListItem{
			Name:   stringifyJSON(widgets.GetValue(item, nameKey)),
			Label:  stringifyJSON(widgets.GetValue(item, labelKey)),
			Target: resolveTemplate(targetTpl, item),
		})
	}
	return result
}

// resolveTemplate replaces `{field}` placeholders in tpl with the matching
// value from item. Unknown placeholders are left intact (so a misconfigured
// mapping surfaces visibly instead of silently producing broken links).
func resolveTemplate(tpl string, item map[string]interface{}) string {
	if tpl == "" || !strings.Contains(tpl, "{") {
		return tpl
	}
	out := tpl
	for k, v := range item {
		placeholder := "{" + k + "}"
		if strings.Contains(out, placeholder) {
			out = strings.ReplaceAll(out, placeholder, stringifyJSON(v))
		}
	}
	return out
}

// stringifyJSON formats a JSON-decoded scalar as a string. Anything that
// isn't a primitive falls through to fmt.Sprint, which is good enough for
// rendering a best-effort value in a list row.
func stringifyJSON(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		// json.Unmarshal decodes all numbers as float64.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(v)
	}
}

// writeProxyError maps proxy errors to clean HTTP responses without
// leaking internal detail.
func writeProxyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, proxyhandlers.ErrInvalidEndpoint):
		http.Error(w, "invalid endpoint", http.StatusBadRequest)
	case errors.Is(err, proxy.ErrSSRFBlocked):
		http.Error(w, "upstream host blocked", http.StatusForbidden)
	case errors.Is(err, proxy.ErrInvalidURL):
		http.Error(w, "invalid upstream URL", http.StatusBadRequest)
	default:
		// Strip any URL fragments from the error message before sending.
		msg := proxy.SanitizeURL(err.Error())
		// Avoid leaking absolute filesystem paths.
		if strings.Contains(msg, "/app/") {
			msg = "upstream error"
		}
		http.Error(w, msg, http.StatusBadGateway)
	}
}

// GetWidgetRegistry returns the default widget registry.
// RegisterBuiltinWidgets() must be called once at startup before using this.
func GetWidgetRegistry() *widgets.Registry {
	return widgets.DefaultRegistry
}

// findServiceWidget finds the widget configuration for a service.
func findServiceWidget(group, service string) *config.WidgetConfig {
	services, err := config.LoadServices()
	if err != nil {
		return nil
	}

	for _, g := range services {
		if g.Name != group {
			continue
		}
		for _, s := range g.Services {
			if s.Name == service {
				return s.Widget
			}
		}
	}
	return nil
}

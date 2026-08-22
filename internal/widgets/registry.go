package widgets

import (
	"context"
	"io"
	"regexp"
	"sort"
	"sync"

	"github.com/thotenn/myserver/internal/config"
)

// Registry holds all registered widgets.
type Registry struct {
	widgets map[string]Widget
	aliases map[string]string
	mu      sync.RWMutex
}

// DefaultRegistry is the global widget registry.
var DefaultRegistry = NewRegistry()

// NewRegistry creates a new widget registry.
func NewRegistry() *Registry {
	return &Registry{
		widgets: make(map[string]Widget),
		aliases: make(map[string]string),
	}
}

// Register adds a widget to the registry.
func (r *Registry) Register(w Widget) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.widgets[w.Name()] = w
}

// RegisterAlias adds an alias for a widget name.
func (r *Registry) RegisterAlias(alias, target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases[alias] = target
}

// Get retrieves a widget by name (resolving aliases).
func (r *Registry) Get(name string) (Widget, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if target, ok := r.aliases[name]; ok {
		name = target
	}

	w, ok := r.widgets[name]
	return w, ok
}

// GetProxyHandler returns the proxy handler for a widget.
func (r *Registry) GetProxyHandler(name string) ProxyHandler {
	w, ok := r.Get(name)
	if !ok {
		return nil
	}
	if bw, ok := w.(*BaseWidget); ok && bw.Proxy != nil {
		return bw.Proxy
	}
	return nil
}

// List returns all registered widget names in sorted order.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.widgets))
	for name := range r.widgets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Has checks if a widget is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.Get(name)
	return ok
}

// WidgetDef describes a built-in widget in declarative form.
type WidgetDef struct {
	Name     string
	API      string
	Mappings map[string]EndpointMapping
}

// builtinWidgets is the declarative registry of all simple built-in widgets.
// Special widgets with custom types (customapi) are registered separately.
var builtinWidgets = []WidgetDef{
	// Priority widgets
	{Name: "docker", API: ""},
	{Name: "glances", API: ""},
	{Name: "resources", API: ""},
	{Name: "speedtest", API: "{url}", Mappings: map[string]EndpointMapping{"status": {Endpoint: ""}}},
	{Name: "photoprism", API: "{url}/api/v1/{endpoint}"},
	{Name: "vikunja", API: "{url}/api/v1/{endpoint}"},

	// Media management
	{Name: "sonarr", API: "{url}/api/v3/{endpoint}?apikey={key}", Mappings: map[string]EndpointMapping{"queue": {Endpoint: "queue"}, "series": {Endpoint: "series"}, "wanted": {Endpoint: "wanted/missing"}, "calendar": {Endpoint: "calendar"}}},
	{Name: "radarr", API: "{url}/api/v3/{endpoint}?apikey={key}", Mappings: map[string]EndpointMapping{"queue": {Endpoint: "queue"}, "movies": {Endpoint: "movie"}, "wanted": {Endpoint: "wanted/missing"}, "calendar": {Endpoint: "calendar"}}},
	{Name: "lidarr", API: "{url}/api/v1/{endpoint}?apikey={key}", Mappings: map[string]EndpointMapping{"queue": {Endpoint: "queue"}, "artists": {Endpoint: "artist"}, "album": {Endpoint: "album"}}},
	{Name: "prowlarr", API: "{url}/api/v1/{endpoint}?apikey={key}", Mappings: map[string]EndpointMapping{"stats": {Endpoint: "stats"}}},
	{Name: "bazarr", API: "{url}/api/{endpoint}", Mappings: map[string]EndpointMapping{"episodes": {Endpoint: "episodes"}, "movies": {Endpoint: "movies"}}},
	{Name: "overseerr", API: "{url}/api/v1/{endpoint}", Mappings: map[string]EndpointMapping{"request": {Endpoint: "request?take=0"}, "movie": {Endpoint: "movie"}, "tv": {Endpoint: "tv"}}},

	// Media servers
	{Name: "plex", API: "{url}{endpoint}?X-Plex-Token={key}", Mappings: map[string]EndpointMapping{"unified": {Endpoint: "/"}, "sessions": {Endpoint: "/status/sessions"}, "libraries": {Endpoint: "/library/sections"}}},
	{Name: "jellyfin", API: "{url}/{endpoint}", Mappings: map[string]EndpointMapping{"info": {Endpoint: "System/Info"}, "sessions": {Endpoint: "Sessions"}, "items": {Endpoint: "Items/Counts"}}},
	{Name: "emby", API: "{url}/emby/{endpoint}", Mappings: map[string]EndpointMapping{"info": {Endpoint: "System/Info"}, "sessions": {Endpoint: "Sessions"}, "items": {Endpoint: "Items/Counts"}}},
	{Name: "tautulli", API: "{url}/api/v2?apikey={key}&cmd={endpoint}", Mappings: map[string]EndpointMapping{"activity": {Endpoint: "get_activity"}, "info": {Endpoint: "get_server_info"}, "libraries": {Endpoint: "get_libraries"}}},

	// Download clients
	{Name: "qbittorrent", API: "{url}/api/v2/{endpoint}", Mappings: map[string]EndpointMapping{"info": {Endpoint: "transfer/info"}, "torrents": {Endpoint: "torrents/info"}}},
	{Name: "transmission", API: "{url}/transmission/rpc", Mappings: map[string]EndpointMapping{"stats": {Endpoint: ""}}},
	{Name: "deluge", API: "{url}/json", Mappings: map[string]EndpointMapping{"stats": {Endpoint: ""}}},
	{Name: "sabnzbd", API: "{url}/api?mode={endpoint}&output=json&apikey={key}", Mappings: map[string]EndpointMapping{"queue": {Endpoint: "queue"}, "history": {Endpoint: "history"}}},

	// Networking
	{Name: "pihole", API: "{url}/admin/api.php?{endpoint}"},
	{Name: "adguard", API: "{url}/control/{endpoint}"},
	{Name: "traefik", API: "{url}/api/{endpoint}"},
	{Name: "caddy", API: "{url}/api/{endpoint}"},
	{Name: "npm", API: "{url}/api/{endpoint}"},
	{Name: "cloudflared", API: "{url}/api/{endpoint}"},
	{Name: "tailscale", API: "{url}/api/v2/{endpoint}"},

	// Monitoring
	{Name: "portainer", API: "{url}/api/{endpoint}", Mappings: map[string]EndpointMapping{"info": {Endpoint: "status"}, "stacks": {Endpoint: "stacks"}, "endpoints": {Endpoint: "endpoints"}}},
	{Name: "uptimekuma", API: "{url}/api/{endpoint}", Mappings: map[string]EndpointMapping{"status": {Endpoint: "status-page"}}},
	{Name: "netdata", API: "{url}/api/v1/{endpoint}", Mappings: map[string]EndpointMapping{"info": {Endpoint: "info"}, "alarms": {Endpoint: "alarms"}, "metrics": {Endpoint: "allmetrics"}}},
	{Name: "prometheus", API: "{url}/api/v1/{endpoint}", Mappings: map[string]EndpointMapping{"query": {Endpoint: "query"}}},
	{Name: "grafana", API: "{url}/api/{endpoint}", Mappings: map[string]EndpointMapping{"stats": {Endpoint: "org"}}},

	// Productivity
	{Name: "nextcloud", API: "{url}/ocs/v2.php/apps/serverinfo/api/v1/{endpoint}"},
	{Name: "trilium", API: "{url}/api/{endpoint}"},
	{Name: "paperlessngx", API: "{url}/api/{endpoint}"},

	// Infrastructure
	{Name: "proxmox", API: "{url}/api2/json/{endpoint}"},
	{Name: "argocd", API: "{url}/api/v1/{endpoint}"},

	// Info widgets
	{Name: "datetime", API: ""},
	{Name: "greeting", API: ""},
	{Name: "search", API: ""},
	{Name: "weather", API: "https://api.openweathermap.org/data/2.5/{endpoint}?lat={latitude}&lon={longitude}&appid={apiKey}&units=metric", Mappings: map[string]EndpointMapping{"current": {Endpoint: "weather"}, "forecast": {Endpoint: "forecast"}}},
	{Name: "openmeteo", API: "https://api.open-meteo.com/v1/{endpoint}?latitude={latitude}&longitude={longitude}&current_weather=true&timezone=auto", Mappings: map[string]EndpointMapping{"current": {Endpoint: "forecast"}}},
	{Name: "stocks", API: ""},
	{Name: "kubernetes", API: ""},
	{Name: "longhorn", API: "{url}/v1/{endpoint}", Mappings: map[string]EndpointMapping{"nodes": {Endpoint: "nodes"}, "volumes": {Endpoint: "volumes"}}},
}

// builtinAliases maps alias names to their canonical widget type.
//
// Every target must be a registered widget: Get() rewrites the name and then
// looks it up, so an alias pointing at nothing resolves to "unknown widget"
// with no hint that the alias itself is the problem. `hoarder` -> `karakeep`
// used to sit here without a `karakeep` widget behind it.
var builtinAliases = map[string]string{
	"jellyseerr":     "overseerr",
	"seerr":          "overseerr",
	"openweathermap": "weather",
}

// nopHandler is a no-op proxy handler for widgets that use the generic handler.
type nopHandler struct{}

func (n *nopHandler) Execute(_ context.Context, _ *config.WidgetConfig, _ string, _ io.Reader) (interface{}, error) {
	return nil, nil
}

// RegisterBuiltinWidgets registers all built-in widgets.
func RegisterBuiltinWidgets() {
	r := DefaultRegistry

	// Register the special customapi widget (has its own type).
	RegisterCustomAPI(r)

	// Register all simple widgets from the declarative slice.
	for _, w := range builtinWidgets {
		r.Register(&simpleWidget{
			typeName: w.Name,
			api:      w.API,
			mappings: w.Mappings,
		})
	}

	// Aliases (backward compatibility)
	for alias, target := range builtinAliases {
		r.RegisterAlias(alias, target)
	}
}

// simpleWidget is a helper to create basic widget definitions.
type simpleWidget struct {
	typeName string
	api      string
	mappings map[string]EndpointMapping
	allowed  *regexp.Regexp
}

func (w *simpleWidget) Name() string                         { return w.typeName }
func (w *simpleWidget) APITemplate() string                  { return w.api }
func (w *simpleWidget) Mappings() map[string]EndpointMapping { return w.mappings }
func (w *simpleWidget) AllowedEndpoints() *regexp.Regexp     { return w.allowed }

// reg registers a simple widget with API template and endpoint mappings.
func reg(r *Registry, typeName, api string, mappings map[string]EndpointMapping) {
	r.Register(&simpleWidget{
		typeName: typeName,
		api:      api,
		mappings: mappings,
	})
}

package widgets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	// Register a widget
	r.Register(&simpleWidget{
		typeName: "test",
		api:      "http://example.com/api/{endpoint}",
		mappings: map[string]EndpointMapping{
			"info": {Endpoint: "info"},
		},
	})

	// Get widget
	w, ok := r.Get("test")
	assert.True(t, ok)
	require.NotNil(t, w)
	assert.Equal(t, "test", w.Name())
	assert.Equal(t, "http://example.com/api/{endpoint}", w.APITemplate())

	// Missing widget
	_, ok = r.Get("missing")
	assert.False(t, ok)
}

func TestRegistryAliases(t *testing.T) {
	r := NewRegistry()

	r.Register(&simpleWidget{
		typeName: "overseerr",
		api:      "{url}/api/v1/{endpoint}",
	})
	r.RegisterAlias("seerr", "overseerr")
	r.RegisterAlias("jellyseerr", "overseerr")

	// Resolve alias
	w, ok := r.Get("seerr")
	assert.True(t, ok)
	assert.Equal(t, "overseerr", w.Name())

	w, ok = r.Get("jellyseerr")
	assert.True(t, ok)
	assert.Equal(t, "overseerr", w.Name())

	// Original name works
	w, ok = r.Get("overseerr")
	assert.True(t, ok)
	assert.Equal(t, "overseerr", w.Name())
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(&simpleWidget{typeName: "alpha"})
	r.Register(&simpleWidget{typeName: "beta"})
	r.Register(&simpleWidget{typeName: "gamma"})

	names := r.List()
	assert.Len(t, names, 3)
	assert.Contains(t, names, "alpha")
	assert.Contains(t, names, "beta")
	assert.Contains(t, names, "gamma")
}

func TestRegistryHas(t *testing.T) {
	r := NewRegistry()
	r.Register(&simpleWidget{typeName: "exists"})

	assert.True(t, r.Has("exists"))
	assert.False(t, r.Has("nope"))
}

func TestBuiltinRegistration(t *testing.T) {
	// Create a fresh registry and register builtins
	r := NewRegistry()

	// Register a few manually (testing the registration functions)
	RegisterCustomAPI(r)
	RegisterPlex(r)
	RegisterSonarr(r)
	RegisterJellyfin(r)
	RegisterOverseerr(r)
	r.RegisterAlias("jellyseerr", "overseerr")

	assert.True(t, r.Has("customapi"))
	assert.True(t, r.Has("plex"))
	assert.True(t, r.Has("sonarr"))
	assert.True(t, r.Has("jellyfin"))
	assert.True(t, r.Has("overseerr"))
	assert.True(t, r.Has("jellyseerr")) // alias

	// Check API templates
	w, _ := r.Get("plex")
	assert.Contains(t, w.APITemplate(), "X-Plex-Token")

	w, _ = r.Get("sonarr")
	assert.Contains(t, w.APITemplate(), "apikey")

	w, _ = r.Get("customapi")
	assert.Equal(t, "{url}", w.APITemplate())
}

func TestEndpointMappings(t *testing.T) {
	r := NewRegistry()
	RegisterGlances(r)

	w, ok := r.Get("glances")
	require.True(t, ok)

	mappings := w.Mappings()
	assert.NotNil(t, mappings)

	_, hasCPU := mappings["cpu"]
	_, hasMemory := mappings["memory"]
	assert.True(t, hasCPU)
	assert.True(t, hasMemory)
}

// Every alias must resolve to a registered widget. An alias pointing at a
// type that was never registered silently degrades to "unknown widget":
// Get() rewrites the name, the lookup misses, and nothing says the alias is
// at fault. `hoarder` -> `karakeep` shipped broken this way.
func TestBuiltinAliasesResolve(t *testing.T) {
	RegisterBuiltinWidgets()

	for alias, target := range builtinAliases {
		if _, ok := DefaultRegistry.Get(target); !ok {
			t.Errorf("alias %q points at %q, which is not a registered widget", alias, target)
		}
		if _, ok := DefaultRegistry.Get(alias); !ok {
			t.Errorf("alias %q does not resolve", alias)
		}
	}
}

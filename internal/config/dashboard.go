package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// A Dashboard is one dashboard being served: the directory it reads, the
// snapshot parsed from it, its authentication policy, and the URL prefix it
// answers under. One process serves many.
//
// This type replaced the package-level globals that used to hold "the" config,
// and they were deleted rather than deprecated on purpose. With more than one
// dashboard in a process, a handler able to reach for "the" config is a
// handler able to serve one client's services to another — or, through the
// widget proxy, to call an upstream with another client's API key. Making the
// dashboard a parameter turns that from a review item into a compile error.
//
// Never copy a Dashboard: the registry hands out pointers and the atomics and
// the once below are part of its identity.
type Dashboard struct {
	// Slug names a client dashboard and is the first path segment it is
	// served under. The root dashboard's slug is "".
	Slug string
	// Prefix is the URL prefix this dashboard answers under: "" for the root
	// dashboard with no base path, the base path for the root dashboard with
	// one, and <basePath>/<slug> for a client.
	Prefix string
	// Dir is the configuration directory this dashboard reads. Nothing else
	// in the process may read it.
	Dir string

	cache atomic.Value // *cachedConfig
	auth  atomic.Value // AuthState

	secretOnce sync.Once
	secret     string
}

// errPolicyNotLoaded is what Auth reports before the first load. It must never
// be mistaken for "no auth configured": that would publish the dashboard.
var errPolicyNotLoaded = errors.New("auth policy not loaded yet")

// NewDashboard builds a dashboard over a config directory. It does not read
// anything: call Reload to populate it.
func NewDashboard(slug, prefix, dir string) *Dashboard {
	return &Dashboard{Slug: slug, Prefix: prefix, Dir: dir}
}

// IsRoot reports whether this is the root dashboard, the only one that owns
// the host, the scripts, the widget proxy and container discovery.
func (d *Dashboard) IsRoot() bool { return d.Slug == "" }

// String is what logs and errors show.
func (d *Dashboard) String() string {
	if d.IsRoot() {
		return "root"
	}
	return d.Slug
}

func (d *Dashboard) snapshot() *cachedConfig {
	c, _ := d.cache.Load().(*cachedConfig)
	return c
}

// Services returns this dashboard's service groups, from the in-memory
// snapshot when there is one and from disk otherwise.
func (d *Dashboard) Services() ([]ServiceGroup, error) {
	if c := d.snapshot(); c != nil {
		return c.Services, nil
	}
	return loadServices(d.Dir)
}

// Bookmarks returns this dashboard's bookmark groups.
func (d *Dashboard) Bookmarks() ([]BookmarkGroup, error) {
	if c := d.snapshot(); c != nil {
		return c.Bookmarks, nil
	}
	return loadBookmarks(d.Dir)
}

// Widgets returns this dashboard's info widgets.
func (d *Dashboard) Widgets() ([]InfoWidget, error) {
	if c := d.snapshot(); c != nil {
		return c.Widgets, nil
	}
	return loadWidgets(d.Dir)
}

// Settings returns this dashboard's settings. It never returns nil without an
// error: callers dereference it straight away.
func (d *Dashboard) Settings() (*Settings, error) {
	if c := d.snapshot(); c != nil && c.Settings != nil {
		return c.Settings, nil
	}
	return loadSettings(d.Dir)
}

// Docker returns this dashboard's docker server definitions. Only the root
// dashboard's are ever used: container discovery is not registered elsewhere.
func (d *Dashboard) Docker() (map[string]DockerConfig, error) {
	if c := d.snapshot(); c != nil {
		return c.Docker, nil
	}
	return loadDocker(d.Dir)
}

// Kubernetes returns this dashboard's kubernetes cluster definitions.
func (d *Dashboard) Kubernetes() (map[string]KubernetesConfig, error) {
	if c := d.snapshot(); c != nil {
		return c.Kubernetes, nil
	}
	return loadKubernetes(d.Dir)
}

// Proxmox returns this dashboard's proxmox node definitions.
func (d *Dashboard) Proxmox() (map[string]ProxmoxConfig, error) {
	if c := d.snapshot(); c != nil {
		return c.Proxmox, nil
	}
	return loadProxmox(d.Dir)
}

// ScriptsFile returns this dashboard's scripts.yaml. Only the root dashboard's
// is ever read: the scripts routes exist on no other.
func (d *Dashboard) ScriptsFile() (*ScriptsFile, error) {
	if c := d.snapshot(); c != nil {
		return c.Scripts, nil
	}
	return loadScriptsFile(d.Dir)
}

// Hash is the config hash the frontend polls to know when to reload. It is
// per dashboard: a change to one client's YAML must not make every other
// dashboard's browser reload.
func (d *Dashboard) Hash() string {
	if c := d.snapshot(); c != nil {
		return c.Hash
	}
	h, _ := configHash(d.Dir)
	return h
}

// ScriptsEnabled reports whether this dashboard may run shell commands. Only
// the root dashboard can, and only when the process opted in: the scripts
// manager runs on the host, so a client dashboard exposing it would be a
// remote shell handed to a third party.
func (d *Dashboard) ScriptsEnabled() bool {
	return d.IsRoot() && ScriptsEnabled()
}

// Auth returns this dashboard's authentication policy. It never returns a
// zero value that means "public" by accident: before the first load it
// reports lockdown.
func (d *Dashboard) Auth() AuthState {
	if s, ok := d.auth.Load().(AuthState); ok {
		return s
	}
	return AuthState{Lockdown: true, Err: errPolicyNotLoaded}
}

// ReloadAuth re-reads this dashboard's auth.yaml and publishes the resulting
// policy atomically. See reconcileAuth for the state table; the short version
// is that no failure path degrades to public.
func (d *Dashboard) ReloadAuth() {
	d.auth.Store(reconcileAuth(d.Auth(), readAuthFile(d.Dir)))
}

// ResetAuthState clears the published policy (for testing). Production code
// never calls it: an empty policy reads as "no auth configured".
func (d *Dashboard) ResetAuthState() {
	d.auth.Store(AuthState{})
}

// ResetCache drops the in-memory snapshot (for testing) so the loaders read
// from disk again.
func (d *Dashboard) ResetCache() {
	d.cache.Store((*cachedConfig)(nil))
}

// Reload re-reads every config file in this dashboard's directory and swaps
// the snapshot atomically. Called at startup and by the watcher.
func (d *Dashboard) Reload() {
	c := &cachedConfig{}
	c.Services, _ = loadServices(d.Dir)
	c.Bookmarks, _ = loadBookmarks(d.Dir)
	c.Widgets, _ = loadWidgets(d.Dir)
	c.Settings, _ = loadSettings(d.Dir)
	c.Docker, _ = loadDocker(d.Dir)
	c.Kubernetes, _ = loadKubernetes(d.Dir)
	c.Proxmox, _ = loadProxmox(d.Dir)
	c.Scripts, _ = loadScriptsFile(d.Dir)
	c.Hash, _ = configHash(d.Dir)
	d.cache.Store(c)
	// auth.yaml deliberately does NOT live in cachedConfig: the pattern above
	// discards load errors, and an auth policy that silently became nil would
	// publish the dashboard. ReloadAuth keeps its own atomic value with
	// last-known-good semantics instead.
	d.ReloadAuth()
}

// SessionSecret returns the key that signs this dashboard's session cookies:
// the configured one, or a random fallback generated once per dashboard.
//
// The fallback is per dashboard and not per process for the reason the whole
// phase exists: one shared key means a cookie minted for one dashboard
// verifies on another, and only the cookie name would be keeping them apart.
func (d *Dashboard) SessionSecret(cfg *AuthConfig) string {
	if cfg != nil && cfg.Session.Secret != "" {
		return cfg.Session.Secret
	}
	d.secretOnce.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			// crypto/rand failing is not recoverable; a predictable signing
			// key would be worse than no sessions at all.
			panic(fmt.Sprintf("generating session secret: %v", err))
		}
		d.secret = hex.EncodeToString(buf)
	})
	return d.secret
}

// CookieName is the name of this dashboard's session cookie.
//
// Dashboards share a hostname, and the session cookie's Path confines where
// the browser SENDS it — not which cookie wins when two of them have the same
// name. A request to /acme would carry both the root's Path=/ cookie and the
// client's Path=/acme one, and the server would read whichever came first. The
// slug goes in the name so that cannot happen. The root dashboard's name is
// unchanged, which is what keeps a single-dashboard deployment identical.
func (d *Dashboard) CookieName(cfg *AuthConfig) string {
	if cfg != nil && cfg.Session.CookieName != "" {
		return cfg.Session.CookieName
	}
	if d.IsRoot() {
		return defaultCookieName
	}
	return defaultCookieName + "_" + d.Slug
}

// CookiePath scopes this dashboard's session cookie. At the root with no base
// path that is "/", exactly as before any of this existed.
func (d *Dashboard) CookiePath() string {
	if d.Prefix != "" {
		return d.Prefix
	}
	return "/"
}

// PrefixPath prepends this dashboard's prefix to a dashboard-relative path.
func (d *Dashboard) PrefixPath(p string) string {
	return PrefixPath(d.Prefix, p)
}

// dashboardKey is unexported so no other package can collide with it.
type dashboardKey struct{}

// WithDashboard attaches the dashboard a request is being served for, along
// with its prefix.
//
// Both go on the context together and from the same source, so the prefix a
// URL is emitted with can never drift from the dashboard the data came from.
func WithDashboard(ctx context.Context, d *Dashboard) context.Context {
	if d == nil {
		return ctx
	}
	return WithBasePath(context.WithValue(ctx, dashboardKey{}, d), d.Prefix)
}

// DashboardFrom returns the dashboard of the request carrying ctx, or nil.
//
// nil has no fallback on purpose. Answering "the root dashboard" for a request
// the edge failed to resolve would reintroduce, as a default, exactly the
// cross-tenant read this design removes; callers fail closed instead.
func DashboardFrom(ctx context.Context) *Dashboard {
	if ctx == nil {
		return nil
	}
	d, _ := ctx.Value(dashboardKey{}).(*Dashboard)
	return d
}

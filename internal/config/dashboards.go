package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
)

// One process serves the root dashboard from the config directory and a
// client dashboard from each sub-directory of config/dashboards/. They are
// told apart by the first path segment, which is why the set below is the
// only thing that maps a URL to a config directory.
//
// The alternative — one process per client — is what this replaced. It needed
// a container, a reverse-proxy rule and a redirect_uri in the identity
// provider per client; this needs a directory.

// DashboardsSubdir is the directory under the root config dir whose
// sub-directories each become a client dashboard.
const DashboardsSubdir = "dashboards"

// reservedSlugs are first path segments the root dashboard already owns. A
// directory named after one of them would shadow the router and is refused at
// scan time rather than silently ignored.
var reservedSlugs = map[string]bool{
	"api":    true,
	"auth":   true,
	"static": true,
}

// DashboardSet is an immutable snapshot of the dashboards being served.
// It is replaced wholesale, never mutated: a request that resolved against
// one set keeps using it even if a rescan lands mid-flight.
type DashboardSet struct {
	root    *Dashboard
	clients map[string]*Dashboard
}

// Root returns the dashboard served from the config directory itself.
func (s *DashboardSet) Root() *Dashboard { return s.root }

// Client returns the dashboard for a slug, or nil.
func (s *DashboardSet) Client(slug string) *Dashboard { return s.clients[slug] }

// Clients returns the client dashboards, ordered by slug.
func (s *DashboardSet) Clients() []*Dashboard {
	out := make([]*Dashboard, 0, len(s.clients))
	for _, d := range s.clients {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// All returns every dashboard, root first.
func (s *DashboardSet) All() []*Dashboard {
	return append([]*Dashboard{s.root}, s.Clients()...)
}

// Resolve returns the dashboard that owns a request path, or nil when the
// path falls outside every prefix this process serves.
//
// It only decides WHICH dashboard; stripping the prefix is the edge's job, so
// that a handler keeps seeing the paths it saw before any of this existed.
func (s *DashboardSet) Resolve(path string) *Dashboard {
	rest, ok := StripPrefix(s.root.Prefix, path)
	if !ok {
		return nil
	}
	if len(s.clients) > 0 {
		if d := s.clients[firstSegment(rest)]; d != nil {
			return d
		}
	}
	return s.root
}

// firstSegment returns the first path segment of an absolute path, "" when
// there is none.
func firstSegment(path string) string {
	p := strings.TrimPrefix(path, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// StripPrefix removes a URL prefix from a path, reporting whether the path was
// inside it. The bare prefix is the dashboard root: `/team` serves what `/`
// serves, with no redirect to `/team/`.
func StripPrefix(prefix, path string) (string, bool) {
	if prefix == "" {
		return path, true
	}
	if path == prefix {
		return "/", true
	}
	if strings.HasPrefix(path, prefix+"/") {
		return path[len(prefix):], true
	}
	return "", false
}

var dashboards atomic.Value // *DashboardSet

// Dashboards returns the current set. It never returns nil once
// InitDashboards has run.
func Dashboards() *DashboardSet {
	s, _ := dashboards.Load().(*DashboardSet)
	return s
}

// SetDashboards publishes a set atomically.
func SetDashboards(s *DashboardSet) { dashboards.Store(s) }

// InitDashboards scans the config tree and publishes the result, returning the
// new set together with the directories it refused. It is safe to call again
// at any time: dashboards that survive the rescan keep their identity, so a
// new client appearing does not drop anyone's cache or invalidate the sessions
// of the dashboards that were already being served.
func InitDashboards() (*DashboardSet, []error) {
	set, errs := ScanDashboards(Dashboards(), ConfigDir(), BasePath())
	SetDashboards(set)
	return set, errs
}

// ScanDashboards builds a set from a root config directory: the root
// dashboard, plus one client per sub-directory of <rootDir>/dashboards.
//
// prev, when not nil, is the set being replaced; a slug present in both keeps
// the SAME *Dashboard. That is not an optimisation. A fresh Dashboard would
// mint a new session-signing key and drop its parsed snapshot, so adding one
// client would sign every other client out and send every open browser back
// to disk.
func ScanDashboards(prev *DashboardSet, rootDir, basePath string) (*DashboardSet, []error) {
	set := &DashboardSet{clients: map[string]*Dashboard{}}

	if prev != nil && prev.root != nil && prev.root.Dir == rootDir && prev.root.Prefix == basePath {
		set.root = prev.root
	} else {
		set.root = NewDashboard("", basePath, rootDir)
	}

	entries, err := os.ReadDir(filepath.Join(rootDir, DashboardsSubdir))
	if err != nil {
		// No dashboards/ directory is the ordinary single-dashboard case, not
		// a failure. Anything else is worth reporting but must not stop the
		// root dashboard from being served.
		if os.IsNotExist(err) {
			return set, nil
		}
		return set, []error{fmt.Errorf("reading %s: %w", DashboardsSubdir, err)}
	}

	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		if err := ValidateSlug(slug); err != nil {
			errs = append(errs, err)
			continue
		}
		dir := filepath.Join(rootDir, DashboardsSubdir, slug)
		prefix := basePath + "/" + slug
		if prev != nil {
			if d := prev.clients[slug]; d != nil && d.Dir == dir && d.Prefix == prefix {
				set.clients[slug] = d
				continue
			}
		}
		set.clients[slug] = NewDashboard(slug, prefix, dir)
	}
	return set, errs
}

// ValidateSlug reports whether a directory name may become a dashboard.
//
// The charset is the base path's, so a slug never needs escaping when it is
// concatenated into a prefix, a cookie name or a redirect. Dot segments and
// separators are rejected outright: a slug is one path segment.
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("dashboard slug is empty")
	}
	if reservedSlugs[slug] {
		return fmt.Errorf("dashboard slug %q is reserved: it would shadow the /%s routes", slug, slug)
	}
	if !basePathSegment.MatchString(slug) {
		return fmt.Errorf("dashboard slug %q must match [A-Za-z0-9._~-]+", slug)
	}
	if slug == "." || slug == ".." {
		return fmt.Errorf("dashboard slug %q is a relative path segment", slug)
	}
	return nil
}

package config

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// A base path lets one instance serve the dashboard under a URL prefix
// (`dashboard.example.com/team`) instead of at the root. The prefix is
// stripped at the edge by middleware.BasePath, so every path a handler sees
// stays relative to the dashboard exactly as it is today; the prefix is added
// back only when a URL is EMITTED, through PrefixPath.
//
// Two accessors, on purpose:
//
//   - BasePath() reads the environment and is memoised. Only handlers.API and
//     cmd/myserver may call it.
//   - BasePathFrom(ctx) reads the prefix of THIS request. Everything else uses
//     it, so that serving more than one dashboard per process becomes a change
//     in who populates the context, not a second migration.

// basePathSegment is one path segment of the prefix. Percent-encoding is
// deliberately not allowed: a prefix is operator-supplied configuration, and
// keeping it to unreserved characters means PrefixPath never has to escape.
var basePathSegment = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

var (
	basePathMu       sync.RWMutex
	basePath         string
	basePathResolved bool
)

// ParseBasePath normalises a URL prefix, returning "" for "no prefix".
//
// A trailing slash is dropped so callers can concatenate unconditionally:
// PrefixPath("/team", "/api/services") is "/team/api/services".
func ParseBasePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" || p == "/" {
		return "", nil
	}
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("base path %q must start with /", raw)
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "", nil
	}
	for _, seg := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		if seg == "" {
			return "", fmt.Errorf("base path %q has an empty segment", raw)
		}
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("base path %q has a relative segment", raw)
		}
		if !basePathSegment.MatchString(seg) {
			return "", fmt.Errorf("base path segment %q must match [A-Za-z0-9._~-]+", seg)
		}
	}
	return p, nil
}

// BasePath returns the process-wide URL prefix from HOMEPAGE_BASE_PATH, or ""
// when unset. An invalid value resolves to "" here; cmd/myserver validates it
// with ParseBasePath at startup and refuses to run, so it never reaches this
// silently.
func BasePath() string {
	basePathMu.RLock()
	if basePathResolved {
		defer basePathMu.RUnlock()
		return basePath
	}
	basePathMu.RUnlock()

	basePathMu.Lock()
	defer basePathMu.Unlock()
	if basePathResolved {
		return basePath
	}
	basePath, _ = ParseBasePath(os.Getenv("HOMEPAGE_BASE_PATH"))
	basePathResolved = true
	return basePath
}

// ResetBasePath clears the memoised value (for testing).
func ResetBasePath() {
	basePathMu.Lock()
	defer basePathMu.Unlock()
	basePath = ""
	basePathResolved = false
}

// basePathKey is unexported so no other package can collide with it.
type basePathKey struct{}

// WithBasePath attaches the prefix this request is served under.
func WithBasePath(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, basePathKey{}, prefix)
}

// BasePathFrom returns the prefix of the request carrying ctx, "" when the
// dashboard is served from the root. It is the accessor every URL-emitting
// call site must use.
func BasePathFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	p, _ := ctx.Value(basePathKey{}).(string)
	return p
}

// PrefixPath prepends a base path to a dashboard-relative absolute path. It is
// the only place prefix and path are joined.
//
// With no prefix it returns p untouched, byte for byte — which is what keeps a
// deployment that does not use this feature identical to the pre-feature one.
func PrefixPath(prefix, p string) string {
	if prefix == "" {
		return p
	}
	switch {
	case p == "", p == "/":
		return prefix + "/"
	case strings.HasPrefix(p, "/"):
		return prefix + p
	default:
		return prefix + "/" + p
	}
}

// PrefixPathFrom is PrefixPath with the prefix taken from the request context.
func PrefixPathFrom(ctx context.Context, p string) string {
	return PrefixPath(BasePathFrom(ctx), p)
}

package config

import (
	"context"
	"testing"
)

func TestParseBasePath(t *testing.T) {
	valid := map[string]string{
		"":              "",
		"/":             "",
		"  ":            "",
		"/team":         "/team",
		"/team/":        "/team",
		"/team//":       "/team",
		" /team ":       "/team",
		"/a/b":          "/a/b",
		"/Team-1_x.y~z": "/Team-1_x.y~z",
	}
	for raw, want := range valid {
		got, err := ParseBasePath(raw)
		if err != nil {
			t.Errorf("ParseBasePath(%q) returned %v, want no error", raw, err)
			continue
		}
		if got != want {
			t.Errorf("ParseBasePath(%q) = %q, want %q", raw, got, want)
		}
	}

	// A prefix is used verbatim in emitted URLs and in a cookie name, so
	// anything that would need escaping, traverse, or split a path is rejected
	// at the door rather than sanitised into something else.
	invalid := []string{
		"team",         // no leading slash
		"/a//b",        // empty segment
		"/..",          // traversal
		"/a/../b",      // traversal
		"/.",           // relative
		"/a b",         // space
		"/a?b",         // query
		"/a#b",         // fragment
		"/a%2Fb",       // percent-encoding
		"/a\\b",        // backslash
		"/a\nb",        // control character
		"/a:b",         // not in the charset
		"/../etc",      // traversal
		"/uno/dos/../", // traversal
	}
	for _, raw := range invalid {
		if got, err := ParseBasePath(raw); err == nil {
			t.Errorf("ParseBasePath(%q) = %q, want an error", raw, got)
		}
	}
}

func TestBasePath_FromEnv(t *testing.T) {
	t.Setenv("HOMEPAGE_BASE_PATH", "/team/")
	ResetBasePath()
	t.Cleanup(ResetBasePath)

	if got := BasePath(); got != "/team" {
		t.Errorf("BasePath() = %q, want %q", got, "/team")
	}
	// Memoised, same as ConfigDir().
	if got := BasePath(); got != "/team" {
		t.Errorf("BasePath() = %q on the second call, want %q", got, "/team")
	}
}

func TestBasePath_UnsetIsEmpty(t *testing.T) {
	t.Setenv("HOMEPAGE_BASE_PATH", "")
	ResetBasePath()
	t.Cleanup(ResetBasePath)

	if got := BasePath(); got != "" {
		t.Errorf("BasePath() = %q with the env var unset, want %q", got, "")
	}
}

func TestBasePathFrom(t *testing.T) {
	if got := BasePathFrom(context.Background()); got != "" {
		t.Errorf("a context with no prefix must report %q, got %q", "", got)
	}
	ctx := WithBasePath(context.Background(), "/team")
	if got := BasePathFrom(ctx); got != "/team" {
		t.Errorf("BasePathFrom = %q, want %q", got, "/team")
	}
	//nolint:staticcheck // a nil context is what an unrendered template can hand us.
	if got := BasePathFrom(nil); got != "" {
		t.Errorf("BasePathFrom(nil) = %q, want %q", got, "")
	}
}

func TestPrefixPath(t *testing.T) {
	cases := []struct{ prefix, path, want string }{
		// With no prefix the path comes back byte for byte: that is the
		// property the unprefixed deployment relies on.
		{"", "/api/services", "/api/services"},
		{"", "/", "/"},
		{"", "", ""},
		{"/team", "/api/services", "/team/api/services"},
		{"/team", "/", "/team/"},
		{"/team", "", "/team/"},
		{"/team", "api/services", "/team/api/services"},
		{"/a/b", "/auth/login?next=%2F", "/a/b/auth/login?next=%2F"},
	}
	for _, c := range cases {
		if got := PrefixPath(c.prefix, c.path); got != c.want {
			t.Errorf("PrefixPath(%q, %q) = %q, want %q", c.prefix, c.path, got, c.want)
		}
	}
}

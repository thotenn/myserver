package templates

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/a-h/templ"
	"github.com/thotenn/myserver/internal/config"
)

// gridColumns carries the configured column count of a service group into
// the stylesheet as the `--service-cols` custom property.
//
// It deliberately does NOT set `grid-template-columns` here. An inline style
// beats every class and every media query, so declaring the grid this way
// forced the desktop layout onto phones: four ~85px columns, in which the
// card name, the status line and the CPU/MEM/RX/TX row all overflowed the
// card. `.service-grid` reads the property back and applies it only from the
// `lg` breakpoint up, capping the count below that.
//
// Returns an empty declaration for a non-positive count, which leaves the
// stylesheet's own default in place.
func gridColumns(columns int) templ.SafeCSS {
	if columns <= 0 {
		return ""
	}
	// SafeCSS skips Templ's CSS sanitiser, which drops custom properties.
	// The value is built from an int, so there is nothing to inject.
	return templ.SafeCSS(fmt.Sprintf("--service-cols: repeat(%d, minmax(0, 1fr));", columns))
}

// barWidth returns a complete CSS declaration `width: N%;` used by the
// progress bars in ResourceBar. Percent is clamped to [0, 100].
func barWidth(percent float64) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return fmt.Sprintf("width: %.1f%%;", percent)
}

// ResourceBarData is the payload used to render ResourcesHTML: a list of
// bars with label, percent, formatted value and CSS bar colour.
type ResourceBarData struct {
	Label    string
	Percent  float64
	Value    string
	BarColor string
}

// backgroundImageURL resolves the user-configured `settings.backgroundImage`
// into a URL safe to embed inside a CSS `url(...)` value:
//
//   - absolute URLs (http, https, data:) are returned verbatim;
//   - everything else is treated as a path relative to the config directory
//     and rewritten to `/api/config/<path>?v=<hash>`, so users can drop an
//     image into `config/` (e.g. `config/wallpaper.jpg`) and reference it
//     just by name. Each segment is percent-encoded.
//
// Inputs that look unsafe (containing line breaks, control chars, quotes,
// `..`, or backslashes) return an empty string so the caller can omit the
// background entirely. This protects against breaking out of the CSS
// quoting context.
func backgroundImageURL(raw, hash string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Reject anything that would prematurely close `url(...)` or break out
	// of the CSS / HTML attribute context. We emit the URL inside an
	// unquoted `url(<value>)` so spaces / parens / commas / quotes are all
	// disallowed verbatim.
	if strings.ContainsAny(raw, "\"'`\\ \n\r\t()") {
		return ""
	}
	if strings.Contains(raw, "..") {
		return ""
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "http://"),
		strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(lower, "data:"):
		return raw
	}
	// Local path — percent-encode each segment so spaces or unicode names
	// remain valid as a URL.
	clean := strings.TrimLeft(raw, "/")
	parts := strings.Split(clean, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	encoded := strings.Join(parts, "/")
	if hash == "" {
		return "/api/config/" + encoded
	}
	return "/api/config/" + encoded + "?v=" + url.QueryEscape(hash)
}

// backgroundStyle returns the full CSS declaration block applied to <body>
// when `settings.backgroundImage` is configured. Returns an empty string
// when no image is set or the value is unsafe — in which case Templ will
// emit `style=""`, which the browser ignores.
//
// The dashboard text colour stays on top of the image via a slight
// translucent overlay applied through the existing `.bg-theme-50 /
// .dark:bg-theme-900` body classes, which already have a non-zero alpha
// from the colour palette.
func backgroundStyle(settings *config.Settings, hash string) templ.SafeCSS {
	if settings == nil {
		return ""
	}
	u := backgroundImageURL(settings.BackgroundImage, hash)
	if u == "" {
		return ""
	}
	// Unquoted `url(<value>)` is valid CSS as long as the value has no
	// whitespace, parens, quotes or commas — all of which are already
	// rejected (or percent-encoded for local paths) by backgroundImageURL.
	//
	// We MUST return unquoted because Templ's pipeline for `style=""`
	// applies HTML-attribute escaping AFTER our value is sanitised, and
	// that step turns `"` into `&amp;#34;` and `'` into `&amp;#39;` —
	// both of which the browser exposes to CSS as literal `&#34;` / `&#39;`
	// text, breaking the URL inside `url(...)`. templ.SafeCSS skips CSS
	// sanitisation but does not skip HTML escaping.
	return templ.SafeCSS(fmt.Sprintf(
		"background-image: url(%s); background-size: cover; background-attachment: fixed; background-position: center; background-repeat: no-repeat;",
		u,
	))
}

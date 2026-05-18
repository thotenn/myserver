package templates

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/thotenn/myserver/internal/config"
)

// iconURL resolves an icon name to a URL.
//
// Supports the same conventions as the Node.js Homepage:
//   - Full URLs (`http://`, `https://`) → returned verbatim.
//   - `mdi:xxx` / `mdi-xxx` → Material Design Icons SVG from jsdelivr.
//   - `si:xxx` → Simple Icons coloured SVG from cdn.simpleicons.org.
//   - `name.png` / `name.svg` → dashboard-icons CDN (now at homarr-labs).
//     Supports PNG, SVG and WEBP — the extension is honored when present.
func iconURL(icon string) string {
	if icon == "" {
		return ""
	}
	// Already a full URL
	if strings.HasPrefix(icon, "https://") || strings.HasPrefix(icon, "http://") {
		return icon
	}
	// Material Design Icons using a dash (mdi-foo) — common in myserver configs.
	if strings.HasPrefix(icon, "mdi-") {
		name := strings.TrimPrefix(icon, "mdi-")
		return fmt.Sprintf("https://cdn.jsdelivr.net/npm/@mdi/svg@latest/svg/%s.svg", url.PathEscape(name))
	}
	// Simple Icons using a dash (si-foo) — common in myserver configs.
	if strings.HasPrefix(icon, "si-") {
		name := strings.TrimPrefix(icon, "si-")
		return fmt.Sprintf("https://cdn.simpleicons.org/%s", url.PathEscape(name))
	}
	// prefixed: mdi:, si:, sh:
	if idx := strings.Index(icon, ":"); idx > 0 && idx < 4 {
		prefix := icon[:idx]
		name := icon[idx+1:]
		switch prefix {
		case "mdi":
			return fmt.Sprintf("https://cdn.jsdelivr.net/npm/@mdi/svg@latest/svg/%s.svg", url.PathEscape(name))
		case "si":
			return fmt.Sprintf("https://cdn.simpleicons.org/%s", url.PathEscape(name))
		case "sh":
			return fmt.Sprintf("https://cdn.jsdelivr.net/npm/simple-icons@latest/icons/%s.svg", url.PathEscape(name))
		}
	}
	// Default: dashboard-icons at homarr-labs (successor of walkxcode/dashboard-icons).
	// Respect the extension if present, otherwise default to png.
	ext := "png"
	name := icon
	if i := strings.LastIndex(icon, "."); i > 0 {
		ext = strings.ToLower(icon[i+1:])
		name = icon[:i]
	}
	// homarr-labs repo uses directories per format: png/, svg/, webp/
	dir := ext
	switch ext {
	case "png", "svg", "webp":
		// supported
	default:
		dir = "png"
		ext = "png"
	}
	return fmt.Sprintf("https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/%s/%s.%s",
		dir, url.PathEscape(name), ext)
}

// defaultBookmarkIcon returns a default Simple-Icons icon name for common
// bookmarks when the user has not set an explicit icon in bookmarks.yaml.
func defaultBookmarkIcon(name string) string {
	switch strings.ToLower(name) {
	case "github":
		return "si:github"
	case "gitlab":
		return "si:gitlab"
	case "stack overflow":
		return "si:stackoverflow"
	case "aws console", "aws":
		return "si:amazonaws"
	case "cloudflare":
		return "si:cloudflare"
	case "vercel":
		return "si:vercel"
	case "postman":
		return "si:postman"
	case "ngrok":
		return "si:ngrok"
	case "localstack":
		return "si:localstack"
	default:
		return ""
	}
}

// resolveBookmarkIcon returns the effective icon for a bookmark, using the
// explicit icon if set or falling back to a default based on the name.
func resolveBookmarkIcon(bm config.Bookmark) string {
	if bm.Icon != "" {
		return bm.Icon
	}
	return defaultBookmarkIcon(bm.Name)
}

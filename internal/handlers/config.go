package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// allowedConfigNames lists exact filenames that may be served from the
// config directory regardless of extension. Used for the legacy
// user-customisation hooks served at top level.
var allowedConfigNames = map[string]bool{
	"custom.css": true,
	"custom.js":  true,
}

// allowedImageExtensions lists image MIME-bearing extensions that may be
// served from the config directory. Lower-cased, leading dot included.
// This lets users drop a wallpaper into `config/` (or a subdirectory) and
// reference it from `settings.yaml: backgroundImage:` by relative path.
var allowedImageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".avif": "image/avif",
	".ico":  "image/x-icon",
	".bmp":  "image/bmp",
}

// ConfigFile serves whitelisted files from the user-managed config
// directory. Two kinds of files are accepted:
//   - The literal entries in allowedConfigNames (custom.css, custom.js).
//   - Any file with an extension in allowedImageExtensions, possibly inside
//     a sub-directory of HOMEPAGE_CONFIG_DIR (so users can group images
//     under e.g. `config/wallpapers/`).
//
// All other paths return 404. The handler is path-traversal safe:
//   - `..` and absolute paths are rejected up front.
//   - After cleaning, the resolved path must remain inside the config
//     directory OF THIS DASHBOARD, which is what stops one client's
//     custom.css request from reading another's.
func ConfigFile(w http.ResponseWriter, r *http.Request) {
	d, ok := dashboardOf(w, r)
	if !ok {
		return
	}
	pathParam := chi.URLParam(r, "path")
	if pathParam == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Explicit path-traversal guard before any filesystem work.
	if strings.Contains(pathParam, "..") ||
		strings.Contains(pathParam, "\\") ||
		filepath.IsAbs(pathParam) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Decide whether this filename is allowed.
	contentType := ""
	if allowedConfigNames[pathParam] {
		contentType = contentTypeFor(pathParam)
	} else {
		ext := strings.ToLower(filepath.Ext(pathParam))
		if ct, ok := allowedImageExtensions[ext]; ok {
			contentType = ct
		}
	}
	if contentType == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	filePath := filepath.Join(d.Dir, pathParam)
	cleanPath := filepath.Clean(filePath)
	cleanDir := filepath.Clean(d.Dir)
	if !strings.HasPrefix(cleanPath, cleanDir+string(filepath.Separator)) &&
		cleanPath != cleanDir {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// For the user-customisation hooks (custom.css, custom.js),
			// missing-file is normal — return empty content so the
			// dashboard `<link>` does not 404. For images, signal not
			// found so the browser falls back to no background.
			if allowedConfigNames[pathParam] {
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write(nil)
				return
			}
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	// Short-circuit aggressive caching so users see updates after editing
	// the file. The dashboard appends a `?v=<hash>` query for cache busting
	// when serving via the `backgroundImage` setting.
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(data)
}

func contentTypeFor(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	}
	return "text/plain"
}

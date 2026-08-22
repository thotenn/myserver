package handlers

import (
	"net/http"

	"github.com/thotenn/myserver/internal/config"
	mw "github.com/thotenn/myserver/internal/middleware"
	"github.com/thotenn/myserver/internal/templates"
)

// Dashboard renders the main dashboard page with server-side templates.
// It reads the config hash via config.CurrentHash() so hot-reloaded changes
// are reflected in the cache-busting query param.
func Dashboard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Load all config
		settings, err := config.LoadSettings()
		if err != nil {
			http.Error(w, "Failed to load settings", http.StatusInternalServerError)
			return
		}

		services, err := config.LoadServices()
		if err != nil {
			http.Error(w, "Failed to load services", http.StatusInternalServerError)
			return
		}

		bookmarks, err := config.LoadBookmarks()
		if err != nil {
			http.Error(w, "Failed to load bookmarks", http.StatusInternalServerError)
			return
		}

		widgets, err := config.LoadWidgets()
		if err != nil {
			http.Error(w, "Failed to load widgets", http.StatusInternalServerError)
			return
		}

		// Determine theme and color
		theme := settings.Theme
		color := settings.Color
		if color == "" {
			color = "slate"
		}

		data := templates.PageData{
			Settings:       settings,
			Services:       services,
			Bookmarks:      bookmarks,
			Widgets:        widgets,
			Theme:          theme,
			Color:          color,
			Language:       settings.Language,
			Hash:           config.CurrentHash(),
			ScriptsEnabled: config.ScriptsEnabled(),
			AuthEmail:      mw.SessionEmail(r.Context()),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.Index(data).Render(r.Context(), w); err != nil {
			http.Error(w, "Template rendering error", http.StatusInternalServerError)
		}
	}
}

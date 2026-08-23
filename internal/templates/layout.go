package templates

import "github.com/thotenn/myserver/internal/config"

// datetimeLocale maps the language setting to a JS Intl locale code used by
// the datetime widget in the browser.
func datetimeLocale(lang string) string {
	if lang == "es" {
		return "es-ES"
	}
	return "en-US"
}

// layoutForGroup returns the layout config for a service group.
func layoutForGroup(settings *config.Settings, groupName string) *config.LayoutGroup {
	if settings == nil || settings.Layout == nil {
		return nil
	}
	if l, ok := settings.Layout[groupName]; ok {
		return &l
	}
	return nil
}

// showSearchBar reports whether the unified search bar should be rendered.
// It is shown when either:
//   - widgets.yaml declares a `search:` entry, OR
//   - settings.yaml has a `quicklaunch:` block (the legacy trigger).
//
// The single SearchWidget then handles both: web-search on Enter and a
// live dropdown that filters services and bookmarks as the user types.
func showSearchBar(data PageData) bool {
	for _, w := range data.Widgets {
		if w.Type == "search" {
			return true
		}
	}
	return data.Settings != nil && data.Settings.QuickLaunch != nil
}

// layoutColumns returns the configured column count for a group, or 0 when
// the group has no layout entry — in which case the stylesheet's own default
// applies.
func layoutColumns(layout *config.LayoutGroup) int {
	if layout == nil || layout.Columns <= 0 {
		return 0
	}
	return layout.Columns
}

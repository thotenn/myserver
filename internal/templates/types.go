package templates

import (
	"github.com/thotenn/myserver/internal/config"
)

// PageData holds all data needed to render the main dashboard page.
type PageData struct {
	Settings       *config.Settings
	Services       []config.ServiceGroup
	Bookmarks      []config.BookmarkGroup
	Widgets        []config.InfoWidget
	Theme          string
	Color          string
	Language       string
	Hash           string
	ScriptsEnabled bool
	// AuthEmail is the signed-in address, empty when authentication is off.
	AuthEmail string
}

// TabGroup represents services organized by tabs.
type TabGroup struct {
	Name   string
	Groups []config.ServiceGroup
}

// DynamicListItem is a single row in a customapi `display: dynamic-list`
// widget. Rendered by DynamicListHTML as a clickable link with an optional
// status label.
type DynamicListItem struct {
	Name   string
	Label  string
	Target string
}

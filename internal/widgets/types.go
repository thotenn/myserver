package widgets

import (
	"context"
	"io"
	"regexp"

	"github.com/thotenn/myserver/internal/config"
)

// Widget defines the interface for a service widget.
// Each widget provides API endpoint definitions and optional custom proxy handling.
type Widget interface {
	// Name returns the widget type name (e.g., "plex", "customapi").
	Name() string

	// APITemplate returns the URL template for the widget's API.
	// Uses {placeholder} syntax for variable substitution.
	APITemplate() string

	// Mappings returns the endpoint mappings for this widget.
	// Key is the endpoint name, value is the mapping config.
	Mappings() map[string]EndpointMapping

	// AllowedEndpoints returns a regex of allowed endpoint patterns.
	// Nil means all endpoints are allowed.
	AllowedEndpoints() *regexp.Regexp
}

// EndpointMapping defines how a widget endpoint maps to an API call.
type EndpointMapping struct {
	Endpoint string            `yaml:"endpoint"`
	Method   string            `yaml:"method,omitempty"`
	Body     interface{}       `yaml:"body,omitempty"`
	Params   []string          `yaml:"params,omitempty"`
	Headers  map[string]string `yaml:"headers,omitempty"`
}

// ProxyHandler handles proxying requests for a widget.
type ProxyHandler interface {
	// Execute performs the proxied request and returns the response data.
	Execute(ctx context.Context, widget *config.WidgetConfig, endpoint string, body io.Reader) (interface{}, error)
}

// Mapper transforms raw API response data into a structured format.
type Mapper interface {
	// Map transforms the raw response data.
	Map(data interface{}) (interface{}, error)
}

// Validator checks if response data is valid.
type Validator interface {
	Validate(data interface{}) bool
}

// BaseWidget provides a default implementation of the Widget interface.
type BaseWidget struct {
	TypeName         string
	API              string
	EndpointMappings map[string]EndpointMapping
	Allowed          *regexp.Regexp
	Proxy            ProxyHandler
	MapFunc          func(interface{}) (interface{}, error)
	ValidateFunc     func(interface{}) bool
}

func (w *BaseWidget) Name() string                         { return w.TypeName }
func (w *BaseWidget) APITemplate() string                  { return w.API }
func (w *BaseWidget) Mappings() map[string]EndpointMapping { return w.EndpointMappings }
func (w *BaseWidget) AllowedEndpoints() *regexp.Regexp     { return w.Allowed }

package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
	"github.com/thotenn/myserver/internal/widgets"
)

// validEndpoint matches an endpoint name supplied by the frontend. It MUST
// be a slash-separated identifier with no path traversal, no scheme, no
// query parameters, no fragment. The frontend is expected to send logical
// endpoint names like "default", "queue", "library/sections" — never raw
// path components.
var validEndpoint = regexp.MustCompile(`^[a-zA-Z0-9_./-]*$`)

// ErrInvalidEndpoint is returned when the endpoint parameter does not match
// the safe pattern (path traversal, query injection, etc).
var ErrInvalidEndpoint = errors.New("invalid endpoint name")

// resolveWidgetAPI queries the default widget registry for the widget type.
// It returns the API template and the resolved endpoint path. If the widget
// type is not registered, it falls back to widget.URL and the raw endpoint.
func resolveWidgetAPI(widget *config.WidgetConfig, endpoint string) (apiTemplate, resolvedEndpoint string) {
	if def, ok := widgets.DefaultRegistry.Get(widget.Type); ok {
		if tpl := def.APITemplate(); tpl != "" {
			apiTemplate = tpl
		} else {
			apiTemplate = widget.URL
		}
		if endpoint != "" && endpoint != "default" {
			if m := def.Mappings(); m != nil {
				if em, ok := m[endpoint]; ok && em.Endpoint != "" {
					resolvedEndpoint = em.Endpoint
				} else {
					resolvedEndpoint = endpoint
				}
			} else {
				resolvedEndpoint = endpoint
			}
		} else {
			resolvedEndpoint = endpoint
		}
		return
	}
	// Fallback: unregistered widget type.
	apiTemplate = widget.URL
	resolvedEndpoint = endpoint
	return
}

// GenericProxyHandler handles widget API requests using the generic pattern.
// It resolves the widget URL, adds authentication, makes the request, and
// returns parsed JSON. The endpoint parameter is validated to prevent path
// traversal and query injection.
//
// When the widget type is registered in the default registry, the handler
// queries the registry for APITemplate and endpoint Mappings instead of
// hard-coding URL construction per type.
//
// file:// scheme is passed through unchanged so widgets can read local JSON
// data directly without an HTTP round-trip.
func GenericProxyHandler(ctx context.Context, widget *config.WidgetConfig, endpoint string, reqBody io.Reader) (interface{}, error) {
	if widget == nil {
		return nil, errors.New("no widget configuration")
	}
	if widget.URL == "" {
		return nil, errors.New("widget url is empty")
	}
	if endpoint != "" && !validEndpoint.MatchString(endpoint) {
		return nil, ErrInvalidEndpoint
	}
	// Reject obvious path traversal attempts even after the regex check.
	if strings.Contains(endpoint, "..") || strings.HasPrefix(endpoint, "/") {
		return nil, ErrInvalidEndpoint
	}

	// file:// scheme — pass through directly without URL construction.
	if strings.HasPrefix(widget.URL, "file://") {
		result, err := proxy.Proxy(ctx, widget.URL, &proxy.Params{Method: http.MethodGet})
		if err != nil {
			return nil, err
		}
		return parseProxyResult(result)
	}

	// Query the widget registry for API template and endpoint mappings.
	apiTemplate, mapping := resolveWidgetAPI(widget, endpoint)
	hasPlaceholder := strings.Contains(apiTemplate, "{")

	var targetURL string
	if hasPlaceholder {
		args := map[string]string{
			"url":      strings.TrimRight(widget.URL, "/"),
			"endpoint": mapping,
		}
		if widget.Key != "" {
			args["key"] = widget.Key
			args["apiKey"] = widget.Key
		}
		if widget.APIKey != "" {
			args["apiKey"] = widget.APIKey
			args["key"] = widget.APIKey
		}
		if widget.Token != "" {
			args["token"] = widget.Token
		}
		if widget.Username != "" {
			args["username"] = widget.Username
		}
		if widget.Password != "" {
			args["password"] = widget.Password
		}
		targetURL = proxy.FormatAPICall(apiTemplate, args)
	} else {
		targetURL = strings.TrimRight(widget.URL, "/")
		if mapping != "" && mapping != "default" {
			targetURL += "/" + strings.TrimLeft(mapping, "/")
		}
	}

	params := &proxy.Params{
		Method:          http.MethodGet,
		Headers:         make(map[string]string),
		FollowRedirects: true,
	}

	if widget.Method != "" {
		params.Method = widget.Method
	}

	// Add authentication. Basic auth wins over Bearer.
	switch {
	case widget.Username != "" && widget.Password != "":
		auth := base64.StdEncoding.EncodeToString([]byte(widget.Username + ":" + widget.Password))
		params.Headers["Authorization"] = "Basic " + auth
	case widget.Key != "" && !hasPlaceholder:
		params.Headers["Authorization"] = "Bearer " + widget.Key
	case widget.APIKey != "" && !hasPlaceholder:
		params.Headers["Authorization"] = "Bearer " + widget.APIKey
	case widget.Token != "" && !hasPlaceholder:
		params.Headers["Authorization"] = "Bearer " + widget.Token
	}

	// Add custom headers from widget config (after auth so widgets can
	// override; trusted because widget config is admin-controlled).
	for k, v := range widget.Headers {
		params.Headers[k] = v
	}

	// Add request body if specified.
	if reqBody != nil {
		params.Body = reqBody
	} else if widget.Body != nil {
		bodyBytes, err := json.Marshal(widget.Body)
		if err != nil {
			return nil, fmt.Errorf("encoding widget body: %w", err)
		}
		params.Body = bytes.NewReader(bodyBytes)
		if params.Headers["Content-Type"] == "" {
			params.Headers["Content-Type"] = "application/json"
		}
	}

	result, err := proxy.Proxy(ctx, targetURL, params)
	if err != nil {
		return nil, err
	}
	return parseProxyResult(result)
}

// parseProxyResult converts a proxy.Result into the JSON data or string
// payload expected by widget consumers.
func parseProxyResult(result *proxy.Result) (interface{}, error) {
	if result.Status == http.StatusNoContent {
		return nil, nil
	}
	if result.Status >= http.StatusBadRequest {
		return nil, fmt.Errorf("upstream returned status %d", result.Status)
	}

	if proxy.IsJSON(result.ContentType) {
		var data interface{}
		if err := json.Unmarshal(result.Body, &data); err != nil {
			return nil, fmt.Errorf("parsing JSON response: %w", err)
		}
		return data, nil
	}

	return string(result.Body), nil
}

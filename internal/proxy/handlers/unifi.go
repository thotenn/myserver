package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
)

// UniFiProxyHandler handles UniFi Controller APIs.
// Uses cookie-based authentication with TLS verification disabled.
func UniFiProxyHandler(ctx context.Context, widget *config.WidgetConfig, endpoint string) (interface{}, error) {
	// Create cookie jar for session
	jar, err := proxy.NewCookieJar()
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	// Step 1: Login
	loginURL := strings.TrimRight(widget.URL, "/") + "/api/login"
	loginBody := fmt.Sprintf(`{"username":"%s","password":"%s"}`, widget.Username, widget.Password)

	_, err = proxy.Proxy(ctx, loginURL, &proxy.Params{
		Method:    http.MethodPost,
		Headers:   map[string]string{"Content-Type": "application/json"},
		Body:      strings.NewReader(loginBody),
		CookieJar: jar,
		IgnoreTLS: true, // UniFi often uses self-signed certs
	})
	if err != nil {
		return nil, fmt.Errorf("unifi login: %w", err)
	}

	// Step 2: API request with session cookie
	apiURL := strings.TrimRight(widget.URL, "/") + endpoint
	result, err := proxy.Proxy(ctx, apiURL, &proxy.Params{
		CookieJar: jar,
		IgnoreTLS: true,
	})
	if err != nil {
		return nil, err
	}

	if result.Status != http.StatusOK {
		return nil, fmt.Errorf("unifi API returned status %d", result.Status)
	}

	var data interface{}
	if err := json.Unmarshal(result.Body, &data); err != nil {
		return nil, fmt.Errorf("parsing unifi response: %w", err)
	}

	return data, nil
}

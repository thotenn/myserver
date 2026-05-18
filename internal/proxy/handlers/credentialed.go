package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
)

// CredentialedProxyHandler handles widgets that require login via form POST.
// It first logs in to get a session cookie, then uses that cookie for the API request.
type CredentialedProxyHandler struct {
	LoginURL    string
	UsernameKey string
	PasswordKey string
	LoginMethod string
}

// NewCredentialedHandler creates a new credentialed proxy handler.
func NewCredentialedHandler(loginURL, usernameKey, passwordKey string) *CredentialedProxyHandler {
	return &CredentialedProxyHandler{
		LoginURL:    loginURL,
		UsernameKey: usernameKey,
		PasswordKey: passwordKey,
		LoginMethod: http.MethodPost,
	}
}

// Execute performs the login and then the actual API request.
func (h *CredentialedProxyHandler) Execute(ctx context.Context, widget *config.WidgetConfig, endpoint string) (interface{}, error) {
	// Create cookie jar for session
	jar, err := proxy.NewCookieJar()
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	// Build login URL
	loginURL := proxy.FormatAPICall(h.LoginURL, map[string]string{
		"url": widget.URL,
	})

	// Login request
	loginBody := url.Values{}
	loginBody.Set(h.UsernameKey, widget.Username)
	loginBody.Set(h.PasswordKey, widget.Password)

	loginParams := &proxy.Params{
		Method:    h.LoginMethod,
		Headers:   map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Body:      strings.NewReader(loginBody.Encode()),
		CookieJar: jar,
	}

	_, err = proxy.Proxy(ctx, loginURL, loginParams)
	if err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}

	// Now make the actual API request with the session cookie
	return GenericProxyHandler(ctx, widget, endpoint, nil)
}

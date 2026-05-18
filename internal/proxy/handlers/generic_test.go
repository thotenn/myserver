package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
)

func TestGenericProxyHandler_ValidRequest(t *testing.T) {
	proxy.SetTestSkipSSRF(true)
	defer proxy.SetTestSkipSSRF(false)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer upstream.Close()

	widget := &config.WidgetConfig{
		Type: "test",
		URL:  upstream.URL,
		Key:  "secret",
	}

	data, err := GenericProxyHandler(context.Background(), widget, "status", nil)
	require.NoError(t, err)
	m, ok := data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ok", m["status"])
}

func TestGenericProxyHandler_InvalidEndpoint(t *testing.T) {
	widget := &config.WidgetConfig{
		Type: "test",
		URL:  "http://example.com",
	}

	_, err := GenericProxyHandler(context.Background(), widget, "../etc/passwd", nil)
	assert.ErrorIs(t, err, ErrInvalidEndpoint)
}

func TestGenericProxyHandler_EmptyURL(t *testing.T) {
	widget := &config.WidgetConfig{Type: "test"}
	_, err := GenericProxyHandler(context.Background(), widget, "status", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url is empty")
}

func TestGenericProxyHandler_PlaceholderSubstitution(t *testing.T) {
	proxy.SetTestSkipSSRF(true)
	defer proxy.SetTestSkipSSRF(false)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The request path should contain the substituted endpoint
		assert.Equal(t, "/api/v1/status", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"path": r.URL.Path})
	}))
	defer upstream.Close()

	widget := &config.WidgetConfig{
		Type: "test",
		URL:  upstream.URL + "/api/v1/{endpoint}",
		Key:  "mykey",
	}

	data, err := GenericProxyHandler(context.Background(), widget, "status", nil)
	require.NoError(t, err)
	m, ok := data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "/api/v1/status", m["path"])
}

func TestGenericProxyHandler_AuthHeaders(t *testing.T) {
	proxy.SetTestSkipSSRF(true)
	defer proxy.SetTestSkipSSRF(false)

	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer upstream.Close()

	tests := []struct {
		name     string
		widget   *config.WidgetConfig
		expected string
	}{
		{
			name:     "basic auth",
			widget:   &config.WidgetConfig{Type: "test", URL: upstream.URL, Username: "user", Password: "pass"},
			expected: "Basic " + basicAuth("user", "pass"),
		},
		{
			name:     "bearer token",
			widget:   &config.WidgetConfig{Type: "test", URL: upstream.URL, Key: "secrettoken"},
			expected: "Bearer secrettoken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedAuth = ""
			_, err := GenericProxyHandler(context.Background(), tt.widget, "", nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, receivedAuth)
		})
	}
}

func TestGenericProxyHandler_CustomHeaders(t *testing.T) {
	proxy.SetTestSkipSSRF(true)
	defer proxy.SetTestSkipSSRF(false)

	var receivedHeader string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer upstream.Close()

	widget := &config.WidgetConfig{
		Type:    "test",
		URL:     upstream.URL,
		Headers: map[string]string{"X-Custom": "value"},
	}

	_, err := GenericProxyHandler(context.Background(), widget, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "value", receivedHeader)
}

func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no sensitive params",
			input:    "http://example.com/api/data?foo=bar",
			expected: "http://example.com/api/data?foo=bar",
		},
		{
			// Both `key` and `apikey` are now considered sensitive — the
			// substring match catches camelCase variants and bare keys.
			name:     "apikey and key both redacted",
			input:    "http://example.com/api?key=abc123&apikey=secret",
			expected: "http://example.com/api?apikey=%5BREDACTED%5D&key=%5BREDACTED%5D",
		},
		{
			name:     "camelCase variants redacted",
			input:    "http://example.com/api?accessToken=t&clientSecret=s",
			expected: "http://example.com/api?accessToken=%5BREDACTED%5D&clientSecret=%5BREDACTED%5D",
		},
		{
			name:     "basic auth user info stripped",
			input:    "https://admin:hunter2@example.com/api",
			expected: "https://%5BREDACTED%5D:%5BREDACTED%5D@example.com/api",
		},
		{
			name:     "token redacted",
			input:    "http://example.com/api?token=mytoken",
			expected: "http://example.com/api?token=%5BREDACTED%5D",
		},
		{
			name:     "multiple sensitive params",
			input:    "http://example.com/api?apikey=secret&token=tk&access_token=at",
			expected: "http://example.com/api?access_token=%5BREDACTED%5D&apikey=%5BREDACTED%5D&token=%5BREDACTED%5D",
		},
		{
			name:     "invalid URL returned as-is",
			input:    "://invalid",
			expected: "://invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatAPICall(t *testing.T) {
	tests := []struct {
		name     string
		template string
		args     map[string]string
		expected string
	}{
		{
			name:     "no placeholders",
			template: "http://example.com/api",
			args:     nil,
			expected: "http://example.com/api",
		},
		{
			name:     "single placeholder",
			template: "http://example.com/api?key={key}",
			args:     map[string]string{"key": "abc123"},
			expected: "http://example.com/api?key=abc123",
		},
		{
			name:     "multiple placeholders",
			template: "{url}/api/{endpoint}?key={key}",
			args: map[string]string{
				"url":      "http://plex:32400",
				"endpoint": "status",
				"key":      "abc",
			},
			expected: "http://plex:32400/api/status?key=abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatAPICall(tt.template, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCache(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	defer cache.Stop()

	// Set and get
	cache.Set("test-key", []byte("test-value"), time.Minute)
	data, ok := cache.Get("test-key")
	assert.True(t, ok)
	assert.Equal(t, []byte("test-value"), data)

	// Missing key
	_, ok = cache.Get("missing-key")
	assert.False(t, ok)

	// Delete
	cache.Delete("test-key")
	_, ok = cache.Get("test-key")
	assert.False(t, ok)
}

func TestCacheKey(t *testing.T) {
	key := CacheKey("plex", "media", "plex-server", 0)
	assert.Equal(t, "plex.media.plex-server.0", key)
}

func TestCachedJSON(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	defer cache.Stop()

	// No cached data
	_, ok := CachedJSON(cache, "test-key")
	assert.False(t, ok)

	// Store JSON
	err := SetJSON(cache, "test-key", map[string]interface{}{"count": 42}, time.Minute)
	require.NoError(t, err)

	// Retrieve
	data, ok := CachedJSON(cache, "test-key")
	assert.True(t, ok)
	assert.Equal(t, float64(42), data["count"])
}

func TestIsJSON(t *testing.T) {
	assert.True(t, IsJSON("application/json"))
	assert.True(t, IsJSON("application/json; charset=utf-8"))
	assert.False(t, IsJSON("text/html"))
	assert.False(t, IsJSON("text/plain"))
}

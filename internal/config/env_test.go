package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubstituteEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "no placeholders",
			input:    "hello world",
			envVars:  nil,
			expected: "hello world",
		},
		{
			name:     "single var",
			input:    "key: {{HOMEPAGE_VAR_TOKEN}}",
			envVars:  map[string]string{"HOMEPAGE_VAR_TOKEN": "abc123"},
			expected: "key: abc123",
		},
		{
			name:     "multiple vars",
			input:    "user: {{HOMEPAGE_VAR_USER}}\npass: {{HOMEPAGE_VAR_PASS}}",
			envVars:  map[string]string{"HOMEPAGE_VAR_USER": "admin", "HOMEPAGE_VAR_PASS": "secret"},
			expected: "user: admin\npass: secret",
		},
		{
			// Missing env vars must NOT be substituted with empty string
			// — the placeholder is kept literally so the error is visible
			// in the rendered YAML.
			name:     "missing var kept literal",
			input:    "key: {{HOMEPAGE_VAR_MISSING}}",
			envVars:  nil,
			expected: "key: {{HOMEPAGE_VAR_MISSING}}",
		},
		{
			// Empty string env var still counts as "defined"; it should be
			// substituted to empty (user opted in explicitly).
			name:     "empty var substitutes to empty",
			input:    "key: {{HOMEPAGE_VAR_EMPTY}}",
			envVars:  map[string]string{"HOMEPAGE_VAR_EMPTY": ""},
			expected: "key: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}
			result := SubstituteEnvVars(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubstituteFileVars(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "homepage_test_*.txt")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := "my-secret-value"
	_, err = tmpFile.WriteString(content)
	assert.NoError(t, err)
	tmpFile.Close()

	os.Setenv("HOMEPAGE_FILE_SECRET", tmpFile.Name())
	defer os.Unsetenv("HOMEPAGE_FILE_SECRET")

	result := SubstituteEnvVars("key: {{HOMEPAGE_FILE_SECRET}}")
	assert.Equal(t, "key: my-secret-value", result)
}

func TestAllowedHosts(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
		isNil    bool
	}{
		{
			name:     "wildcard allows all",
			envValue: "*",
			isNil:    true,
		},
		{
			name:     "empty allows all",
			envValue: "",
			isNil:    true,
		},
		{
			name:     "single host",
			envValue: "example.com",
			expected: []string{"example.com"},
		},
		{
			name:     "multiple hosts",
			envValue: "example.com, test.com",
			expected: []string{"example.com", "test.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("HOMEPAGE_ALLOWED_HOSTS", tt.envValue)
			defer os.Unsetenv("HOMEPAGE_ALLOWED_HOSTS")

			// Reset the once to allow re-reading
			result := AllowedHosts()
			if tt.isNil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestScriptsEnabled(t *testing.T) {
	os.Unsetenv("HOMEPAGE_SCRIPTS_ENABLED")
	assert.False(t, ScriptsEnabled())

	os.Setenv("HOMEPAGE_SCRIPTS_ENABLED", "true")
	assert.True(t, ScriptsEnabled())
	os.Unsetenv("HOMEPAGE_SCRIPTS_ENABLED")
}

func TestProxyDisableIPv6(t *testing.T) {
	os.Unsetenv("HOMEPAGE_PROXY_DISABLE_IPV6")
	assert.False(t, ProxyDisableIPv6())

	os.Setenv("HOMEPAGE_PROXY_DISABLE_IPV6", "true")
	assert.True(t, ProxyDisableIPv6())
	os.Unsetenv("HOMEPAGE_PROXY_DISABLE_IPV6")
}

func TestSubstituteEnvVars_Adversarial(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		setup    func(t *testing.T)
		expected string
	}{
		{
			name:     "empty variable name",
			input:    "key: {{HOMEPAGE_VAR_}}",
			setup:    func(t *testing.T) {},
			expected: "key: {{HOMEPAGE_VAR_}}",
		},
		{
			name:     "nested placeholders",
			input:    "key: {{HOMEPAGE_VAR_FOO{{HOMEPAGE_VAR_BAR}}",
			setup:    func(t *testing.T) {},
			expected: "key: {{HOMEPAGE_VAR_FOO{{HOMEPAGE_VAR_BAR}}",
		},
		{
			name:     "unclosed placeholder",
			input:    "key: {{HOMEPAGE_VAR_FOO",
			setup:    func(t *testing.T) {},
			expected: "key: {{HOMEPAGE_VAR_FOO",
		},
		{
			name:     "malformed prefix",
			input:    "key: {{HOMEPAGE_VAR_ FOO}}",
			setup:    func(t *testing.T) {},
			expected: "key: {{HOMEPAGE_VAR_ FOO}}",
		},
		{
			name:     "very long value",
			input:    "key: {{HOMEPAGE_VAR_LONG}}",
			setup:    func(t *testing.T) { os.Setenv("HOMEPAGE_VAR_LONG", strings.Repeat("x", 10000)) },
			expected: "key: " + strings.Repeat("x", 10000),
		},
		{
			name:     "binary-like content without null",
			input:    "key: {{HOMEPAGE_VAR_BINARY}}",
			setup:    func(t *testing.T) { os.Setenv("HOMEPAGE_VAR_BINARY", "\x01\x02\x03\x04") },
			expected: "key: \x01\x02\x03\x04",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			defer func() {
				// Clean up any env vars set by setup
				for _, e := range os.Environ() {
					if strings.HasPrefix(e, "HOMEPAGE_VAR_") {
						parts := strings.SplitN(e, "=", 2)
						os.Unsetenv(parts[0])
					}
				}
			}()
			result := SubstituteEnvVars(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubstituteFileVars_MissingFile(t *testing.T) {
	os.Setenv("HOMEPAGE_FILE_MISSING", "/nonexistent/path/to/file")
	defer os.Unsetenv("HOMEPAGE_FILE_MISSING")

	result := SubstituteEnvVars("key: {{HOMEPAGE_FILE_MISSING}}")
	// Missing file should keep the placeholder literal
	assert.Equal(t, "key: {{HOMEPAGE_FILE_MISSING}}", result)
}

func TestSubstituteFileVars_LargeFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "homepage_large_*.txt")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := strings.Repeat("a", 1<<20) // 1 MB
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	os.Setenv("HOMEPAGE_FILE_LARGE", tmpFile.Name())
	defer os.Unsetenv("HOMEPAGE_FILE_LARGE")

	result := SubstituteEnvVars("key: {{HOMEPAGE_FILE_LARGE}}")
	assert.Equal(t, "key: "+content, result)
}

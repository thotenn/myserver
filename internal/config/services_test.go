package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadServices(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `- Infrastructure:
    - Grafana:
        href: https://grafana.example.com
        icon: mdi:chart-areaspline
        description: Metrics dashboard
    - PostgreSQL:
        href: https://pgsql.example.com
        icon: mdi:database
        widget:
          type: customapi
          url: http://localhost:5432
          key: test-key
        weight: 10

- Scripts:
    - Backup:
        icon: mdi:backup
        description: Run backup
        type: script
        script: backup
        requireConfirm: true
`

	err := os.WriteFile(filepath.Join(tmpDir, "services.yaml"), []byte(yamlContent), 0644)
	require.NoError(t, err)

	groups, err := loadServices(tmpDir)
	require.NoError(t, err)
	require.Len(t, groups, 2)

	assert.Equal(t, "Infrastructure", groups[0].Name)
	require.Len(t, groups[0].Services, 2)
	assert.Equal(t, "Grafana", groups[0].Services[0].Name)
	assert.Equal(t, "https://grafana.example.com", groups[0].Services[0].Href)
	assert.Equal(t, "mdi:chart-areaspline", groups[0].Services[0].Icon)

	assert.Equal(t, "PostgreSQL", groups[0].Services[1].Name)
	require.NotNil(t, groups[0].Services[1].Widget)
	assert.Equal(t, "customapi", groups[0].Services[1].Widget.Type)
	assert.Equal(t, 10, groups[0].Services[1].Weight)

	assert.Equal(t, "Scripts", groups[1].Name)
	assert.Equal(t, "script", groups[1].Services[0].Type)
	assert.Equal(t, "backup", groups[1].Services[0].Script)
	assert.True(t, groups[1].Services[0].RequireConfirm)
}

func TestSanitizeService(t *testing.T) {
	svc := Service{
		Name: "Test",
		Widget: &WidgetConfig{
			Type:     "plex",
			URL:      "http://localhost:32400",
			Key:      "super-secret-key",
			Username: "admin",
			Password: "secret",
		},
	}

	clean := SanitizeService(svc)
	assert.Equal(t, "Test", clean.Name)
	assert.NotNil(t, clean.Widget)
	assert.Equal(t, "plex", clean.Widget.Type)
	assert.Equal(t, "http://localhost:32400", clean.Widget.URL)
	assert.Empty(t, clean.Widget.Key)
	assert.Empty(t, clean.Widget.Username)
	assert.Empty(t, clean.Widget.Password)
}

// Labels are free-form operator text and were previously published verbatim,
// so a secret written there leaked through /api/services.
func TestSanitizeService_Labels(t *testing.T) {
	in := Service{
		Name: "App",
		Labels: map[string]string{
			"api_key":     "hunter2",
			"token":       "abc123",
			"PASSWORD":    "s3cr3t",
			"environment": "production",
			"owner":       "platform-team",
			"dashboard":   "https://user:pw@dash.example.com/x?apikey=leak",
			"note":        "a:b{c} not a url",
		},
	}

	out := SanitizeService(in)

	for _, secret := range []string{"hunter2", "abc123", "s3cr3t", "pw", "leak"} {
		for k, v := range out.Labels {
			if strings.Contains(v, secret) {
				t.Errorf("label %q leaked %q: %q", k, secret, v)
			}
		}
	}
	for _, k := range []string{"api_key", "token", "PASSWORD"} {
		if _, ok := out.Labels[k]; ok {
			t.Errorf("sensitive label %q should have been dropped", k)
		}
	}
	if out.Labels["environment"] != "production" || out.Labels["owner"] != "platform-team" {
		t.Errorf("harmless labels must survive: %+v", out.Labels)
	}
	// Non-URL values must come back byte for byte: round-tripping arbitrary
	// text through url.Parse would rewrite escapes.
	if out.Labels["note"] != "a:b{c} not a url" {
		t.Errorf("non-URL label was rewritten: %q", out.Labels["note"])
	}
	if got := out.Labels["dashboard"]; got != "https://dash.example.com/x" {
		t.Errorf("URL label = %q, want credentials stripped", got)
	}

	// The caller's map is shared with the config cache and the widget proxy.
	if in.Labels["api_key"] != "hunter2" {
		t.Error("sanitizing must not mutate the input map")
	}
	if len(in.Labels) != 7 {
		t.Errorf("input map lost entries: %d", len(in.Labels))
	}
}

func TestSanitizeService_NilLabels(t *testing.T) {
	out := SanitizeService(Service{Name: "App"})
	if out.Labels != nil {
		t.Errorf("nil labels must stay nil, got %+v", out.Labels)
	}
}

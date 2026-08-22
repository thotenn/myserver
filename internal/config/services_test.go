package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadServices(t *testing.T) {
	tmpDir := t.TempDir()
	SetConfigDir(tmpDir)
	defer ResetConfigDir()

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

	groups, err := LoadServices()
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

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSettings(t *testing.T) {
	tmpDir := t.TempDir()
	SetConfigDir(tmpDir)
	defer ResetConfigDir()

	yamlContent := `title: Mi Dashboard
theme: dark
color: slate
language: es
target: _blank
headerStyle: boxed
cardBlur: true
layout:
  Infraestructura:
    style: row
    columns: 3
  Aplicaciones:
    style: row
    columns: 4
quicklaunch:
  searchDescriptions: true
`

	err := os.WriteFile(filepath.Join(tmpDir, "settings.yaml"), []byte(yamlContent), 0644)
	require.NoError(t, err)

	settings, err := LoadSettings()
	require.NoError(t, err)

	assert.Equal(t, "Mi Dashboard", settings.Title)
	assert.Equal(t, "dark", settings.Theme)
	assert.Equal(t, "slate", settings.Color)
	assert.Equal(t, "es", settings.Language)
	assert.Equal(t, "_blank", settings.Target)
	assert.Equal(t, "boxed", settings.HeaderStyle)
	assert.True(t, settings.CardBlur)

	require.NotNil(t, settings.Layout)
	assert.Equal(t, 3, settings.Layout["Infraestructura"].Columns)
	assert.Equal(t, 4, settings.Layout["Aplicaciones"].Columns)

	require.NotNil(t, settings.QuickLaunch)
	assert.True(t, settings.QuickLaunch.SearchDescriptions)
}

func TestLoadSettingsDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	SetConfigDir(tmpDir)
	defer ResetConfigDir()

	err := os.WriteFile(filepath.Join(tmpDir, "settings.yaml"), []byte("title: Test"), 0644)
	require.NoError(t, err)

	settings, err := LoadSettings()
	require.NoError(t, err)

	assert.Equal(t, "dark", settings.Theme)
	assert.Equal(t, "slate", settings.Color)
	assert.Equal(t, "en", settings.Language)
	assert.Equal(t, "_blank", settings.Target)
	assert.Equal(t, "underlined", settings.HeaderStyle)
}

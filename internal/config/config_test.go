package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckAndCopyConfig(t *testing.T) {
	tmpDir := t.TempDir()
	SetConfigDir(tmpDir)
	defer ResetConfigDir()

	err := CheckAndCopyConfig("settings.yaml")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tmpDir, "settings.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "title")

	err = CheckAndCopyConfig("settings.yaml")
	require.NoError(t, err)
}

func TestConfigHash(t *testing.T) {
	tmpDir := t.TempDir()
	SetConfigDir(tmpDir)
	defer ResetConfigDir()

	for _, f := range []string{"services.yaml", "bookmarks.yaml", "widgets.yaml",
		"docker.yaml", "kubernetes.yaml", "proxmox.yaml", "settings.yaml", "scripts.yaml"} {
		err := CheckAndCopyConfig(f)
		require.NoError(t, err)
	}

	hash1, err := ConfigHash()
	require.NoError(t, err)
	assert.Len(t, hash1, 16)

	hash2, err := ConfigHash()
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	err = os.WriteFile(filepath.Join(tmpDir, "settings.yaml"), []byte("title: changed"), 0644)
	require.NoError(t, err)

	hash3, err := ConfigHash()
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

func TestReadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	SetConfigDir(tmpDir)
	defer ResetConfigDir()

	testContent := "title: Test\ncolor: red\n{{HOMEPAGE_VAR_TEST}}"
	err := os.WriteFile(filepath.Join(tmpDir, "test.yaml"), []byte(testContent), 0644)
	require.NoError(t, err)

	os.Setenv("HOMEPAGE_VAR_TEST", "expanded")
	defer os.Unsetenv("HOMEPAGE_VAR_TEST")

	data, err := ReadConfigFile("test.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(data), "expanded")
	assert.NotContains(t, string(data), "{{HOMEPAGE_VAR_TEST}}")
}

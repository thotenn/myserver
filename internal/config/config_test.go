package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureSkeleton(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, EnsureSkeleton(tmpDir))

	data, err := os.ReadFile(filepath.Join(tmpDir, "settings.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "title")

	// Seeding twice must not overwrite what the operator edited.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "settings.yaml"), []byte("title: mine"), 0644))
	require.NoError(t, EnsureSkeleton(tmpDir))
	data, err = os.ReadFile(filepath.Join(tmpDir, "settings.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "title: mine", string(data))
}

func TestConfigHash(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, EnsureSkeleton(tmpDir))

	hash1, err := configHash(tmpDir)
	require.NoError(t, err)
	assert.Len(t, hash1, 16)

	hash2, err := configHash(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	err = os.WriteFile(filepath.Join(tmpDir, "settings.yaml"), []byte("title: changed"), 0644)
	require.NoError(t, err)

	hash3, err := configHash(tmpDir)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

// Two dashboards must not share a hash, or a change to one would make the
// other's browser reload — and, worse, would look like a change to its config.
func TestConfigHash_IsPerDirectory(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(a, "services.yaml"), []byte("- A: []"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(b, "services.yaml"), []byte("- B: []"), 0644))

	hashA, err := configHash(a)
	require.NoError(t, err)
	hashB, err := configHash(b)
	require.NoError(t, err)
	assert.NotEqual(t, hashA, hashB)
}

func TestReadConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	testContent := "title: Test\ncolor: red\n{{HOMEPAGE_VAR_TEST}}"
	err := os.WriteFile(filepath.Join(tmpDir, "test.yaml"), []byte(testContent), 0644)
	require.NoError(t, err)

	t.Setenv("HOMEPAGE_VAR_TEST", "expanded")

	data, err := readConfigFile(tmpDir, "test.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(data), "expanded")
	assert.NotContains(t, string(data), "{{HOMEPAGE_VAR_TEST}}")
}

// A dashboard directory holds only the files its operator wrote. A missing one
// is "nothing configured", not an error: that is what lets a client dashboard
// be a services.yaml and nothing else.
func TestReadConfigFile_MissingIsEmpty(t *testing.T) {
	data, err := readConfigFile(t.TempDir(), "services.yaml")
	require.NoError(t, err)
	assert.Nil(t, data)
}

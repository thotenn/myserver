package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBookmarks(t *testing.T) {
	tmpDir := t.TempDir()
	SetConfigDir(tmpDir)
	defer ResetConfigDir()

	yamlContent := `- Developer:
    - Github:
        abbr: GH
        href: https://github.com
    - Stack Overflow:
        abbr: SO
        href: https://stackoverflow.com

- Social:
    - Reddit:
        abbr: Re
        href: https://reddit.com
`

	err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yaml"), []byte(yamlContent), 0644)
	require.NoError(t, err)

	groups, err := LoadBookmarks()
	require.NoError(t, err)
	require.Len(t, groups, 2)

	assert.Equal(t, "Developer", groups[0].Name)
	require.Len(t, groups[0].Bookmarks, 2)
	assert.Equal(t, "Github", groups[0].Bookmarks[0].Name)
	assert.Equal(t, "GH", groups[0].Bookmarks[0].Abbr)
	assert.Equal(t, "https://github.com", groups[0].Bookmarks[0].Href)
}

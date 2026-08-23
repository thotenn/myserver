package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

// The whole point of AssetVersion is that editing a built asset changes it.
// The `?v=` used to be config.CurrentHash(), which does not move when a deploy
// ships a new stylesheet — browsers then served a day-old CSS against new
// markup, which looks like a broken layout rather than a stale cache.
func TestHashStaticAssets_ChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "css")
	if err := os.MkdirAll(css, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(css, "main.css"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(".a{color:red}")
	before := hashStaticAssets(dir)

	if again := hashStaticAssets(dir); again != before {
		t.Errorf("hash is not stable across calls: %q then %q", before, again)
	}

	write(".a{color:blue}")
	if after := hashStaticAssets(dir); after == before {
		t.Errorf("editing an asset did not change the version (still %q)", before)
	}
}

// A rename with identical content must also bust the cache: the URL changes,
// and a browser holding the old path would otherwise keep a file that no
// longer exists.
func TestHashStaticAssets_ChangesWithPath(t *testing.T) {
	body := []byte("body{}")

	dirA := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "main.css"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirB, "app.css"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	// Both trees hold one file with identical bytes; only the name differs.
	if a, b := hashStaticAssets(dirA), hashStaticAssets(dirB); a == b {
		t.Errorf("a renamed asset produced the same version %q", a)
	}
}

// A missing tree means the process was started from the wrong directory. The
// page must still render, uncached, rather than fail.
func TestHashStaticAssets_MissingTree(t *testing.T) {
	if got := hashStaticAssets(filepath.Join(t.TempDir(), "nope")); got != "dev" {
		t.Errorf("expected the \"dev\" fallback for an unreadable tree, got %q", got)
	}
}

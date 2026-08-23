package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// staticRoot is the directory served under /static/. It is the build output
// (compiled Tailwind CSS, app.js, locales), not user configuration.
const staticRoot = "web/static"

var (
	assetVersionOnce sync.Once
	assetVersionVal  string
)

// AssetVersion returns a short content hash of the compiled frontend assets,
// used as the `?v=` on the CSS and JS links.
//
// It exists because that parameter used to carry config.CurrentHash(), which
// is derived from the user's YAML files ALONE. A deploy that shipped a new
// main.css without touching the config produced the identical URL, so browsers
// kept serving the previous stylesheet — cached for `max-age=86400` — against
// freshly rendered markup. New class names met an old file, and the result
// reads as a broken layout rather than as a caching problem, which is what
// makes it expensive to diagnose.
//
// Keep this separate from config.CurrentHash(): that one drives the
// `config-hash` meta tag and the /api/hash reload poll, and must keep tracking
// the config and only the config.
//
// Computed once — the assets are baked into the image and cannot change while
// the process runs.
func AssetVersion() string {
	assetVersionOnce.Do(func() {
		assetVersionVal = hashStaticAssets(staticRoot)
	})
	return assetVersionVal
}

// hashStaticAssets hashes every file under root, path included so that a
// rename alone changes the result. WalkDir visits lexically, so the hash is
// stable across runs and machines.
//
// Returns "dev" when the tree cannot be read: a missing static dir means the
// process was started from the wrong working directory, and a page that still
// renders (uncached) is more useful than one that fails.
func hashStaticAssets(root string) string {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.WriteString(h, filepath.ToSlash(path)); err != nil {
			return err
		}
		_, err = io.Copy(h, f)
		return err
	})
	if err != nil {
		return "dev"
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

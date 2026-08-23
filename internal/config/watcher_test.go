package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

// waitFor polls until cond holds or the deadline passes. The watcher is
// asynchronous, so the alternative is a fixed sleep that is either flaky or
// slow.
func waitFor(t *testing.T, why string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", why)
}

func startWatcher(t *testing.T, root string) chan *Dashboard {
	t.Helper()
	SetConfigDir(root)
	ResetBasePath()
	t.Cleanup(func() {
		ResetConfigDir()
		ResetBasePath()
		SetDashboards(nil)
	})
	set, errs := InitDashboards()
	if len(errs) > 0 {
		t.Fatalf("scanning dashboards: %v", errs)
	}
	for _, d := range set.All() {
		d.Reload()
	}

	w, err := NewWatcher(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan *Dashboard, 16)
	if err := w.Start(func(d *Dashboard) {
		select {
		case changed <- d:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Stop() })
	return changed
}

// A directory that appears while the process is running is served without a
// restart. That is what makes adding a client one step instead of three.
func TestWatcher_AddsADashboardAtRuntime(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DashboardsSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	startWatcher(t, root)

	dir := filepath.Join(root, DashboardsSubdir, "acme")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "services.yaml"),
		[]byte("- Acme:\n    - Wiki:\n        href: https://wiki.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "the new dashboard to be registered", func() bool {
		d := Dashboards().Client("acme")
		if d == nil {
			return false
		}
		groups, err := d.Services()
		return err == nil && len(groups) == 1
	})
}

// A dashboard directory whose name ends in a config-file extension is a real
// name, not a config file. Deciding by extension made "reports.js" appear only
// after a restart — served correctly, but never hot-added.
func TestWatcher_AddsADashboardWhoseNameLooksLikeAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DashboardsSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	startWatcher(t, root)

	for _, slug := range []string{"reports.js", "notes.yaml", "theme.css"} {
		if err := os.MkdirAll(filepath.Join(root, DashboardsSubdir, slug), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	waitFor(t, "every dashboard to be registered", func() bool {
		set := Dashboards()
		return set.Client("reports.js") != nil &&
			set.Client("notes.yaml") != nil &&
			set.Client("theme.css") != nil
	})
}

// A change inside one dashboard reloads that one and nobody else: the hash the
// frontend polls is per dashboard, so a shared reload would tell every other
// dashboard's browser to refresh for a change it cannot see.
func TestWatcher_ReloadsOnlyTheDashboardThatChanged(t *testing.T) {
	root := t.TempDir()
	acme := filepath.Join(root, DashboardsSubdir, "acme")
	if err := os.MkdirAll(acme, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "services.yaml"), []byte("- Root: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acme, "services.yaml"), []byte("- Acme: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed := startWatcher(t, root)

	rootHash := Dashboards().Root().Hash()

	if err := os.WriteFile(filepath.Join(acme, "services.yaml"),
		[]byte("- Acme:\n    - Wiki:\n        href: https://wiki.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case d := <-changed:
		if d.Slug != "acme" {
			t.Fatalf("reloaded %q, want the dashboard that changed", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the watcher never reported the change")
	}

	if got := Dashboards().Root().Hash(); got != rootHash {
		t.Error("a change to one dashboard moved another's config hash, which " +
			"would tell its browsers to reload for a change they cannot see")
	}
}

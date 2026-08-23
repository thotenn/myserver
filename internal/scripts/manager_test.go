package scripts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to create a tmp scripts dir with a working hello.sh
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	hello := filepath.Join(dir, "hello.sh")
	require.NoError(t, os.WriteFile(hello, []byte("#!/bin/bash\necho hello\n"), 0o755))
	mgr := NewManager([]string{dir}, 5, 30, 5)
	return mgr, dir
}

func TestRegister_HappyPath(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Register(&ScriptConfig{
		Name:    "hello",
		Command: "hello.sh",
	})
	require.NoError(t, err)
	assert.True(t, mgr.Has("hello"))
}

func TestRegister_ScriptDirBehindASymlink(t *testing.T) {
	// A scriptDir that is itself reached through a symlink must still work.
	// The containment check compares the resolved script path against the
	// resolved directory; comparing it against the UNRESOLVED one reported a
	// traversal for every path under a symlinked parent — which is what every
	// temp dir on macOS is (/var -> /private/var), and what a deployment that
	// bind-mounts its scripts through a symlinked path would hit too.
	real := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(real, "hello.sh"),
		[]byte("#!/bin/bash\necho hello\n"), 0o755))
	link := filepath.Join(t.TempDir(), "scripts-link")
	require.NoError(t, os.Symlink(real, link))

	mgr := NewManager([]string{link}, 5, 30, 5)
	require.NoError(t, mgr.Register(&ScriptConfig{Name: "hello", Command: "hello.sh"}))
	assert.True(t, mgr.Has("hello"))

	// And the escape is still refused through the symlinked dir.
	outside := filepath.Join(t.TempDir(), "evil.sh")
	require.NoError(t, os.WriteFile(outside, []byte("#!/bin/bash\n"), 0o755))
	err := mgr.Register(&ScriptConfig{Name: "evil", Command: "../" + filepath.Base(filepath.Dir(outside)) + "/evil.sh"})
	assert.Error(t, err, "a path resolving outside the symlinked dir must still be refused")
}

func TestValidateScript_RejectsAbsolutePath(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Register(&ScriptConfig{
		Name:    "evil",
		Command: "/etc/passwd",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestValidateScript_RejectsPathTraversal(t *testing.T) {
	mgr, _ := newTestManager(t)
	err := mgr.Register(&ScriptConfig{
		Name:    "evil",
		Command: "../../etc/passwd",
	})
	require.Error(t, err)
}

func TestValidateScript_RejectsSymlinkOutsideDir(t *testing.T) {
	dir := t.TempDir()
	// outside file
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "evil.sh")
	require.NoError(t, os.WriteFile(outsideFile, []byte("#!/bin/bash\n"), 0o755))
	// symlink inside scriptDir pointing outside
	link := filepath.Join(dir, "evil.sh")
	require.NoError(t, os.Symlink(outsideFile, link))

	mgr := NewManager([]string{dir}, 5, 30, 5)
	err := mgr.Register(&ScriptConfig{
		Name:    "evil",
		Command: "evil.sh",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestValidateScript_RejectsNonShExtension(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "evil.py")
	require.NoError(t, os.WriteFile(pyFile, []byte("print('x')\n"), 0o755))

	mgr := NewManager([]string{dir}, 5, 30, 5)
	err := mgr.Register(&ScriptConfig{
		Name:    "evil",
		Command: "evil.py",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only .sh")
}

func TestValidateScript_RejectsShBak(t *testing.T) {
	dir := t.TempDir()
	bakFile := filepath.Join(dir, "thing.sh.bak")
	require.NoError(t, os.WriteFile(bakFile, []byte("x"), 0o755))
	mgr := NewManager([]string{dir}, 5, 30, 5)
	err := mgr.Register(&ScriptConfig{Name: "x", Command: "thing.sh.bak"})
	require.Error(t, err)
}

func TestValidateScript_RejectsPrefixCollision(t *testing.T) {
	// Two sibling dirs that share a prefix: /tmp/scripts and /tmp/scriptsbak
	parent := t.TempDir()
	scripts := filepath.Join(parent, "scripts")
	scriptsBak := filepath.Join(parent, "scriptsbak")
	require.NoError(t, os.Mkdir(scripts, 0o755))
	require.NoError(t, os.Mkdir(scriptsBak, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scriptsBak, "x.sh"), []byte("x"), 0o755))

	mgr := NewManager([]string{scripts}, 5, 30, 5)
	// Attempt to escape into the sibling via "../scriptsbak/x.sh"
	err := mgr.Register(&ScriptConfig{Name: "x", Command: "../scriptsbak/x.sh"})
	require.Error(t, err)
}

func TestValidateScript_RejectsDangerousEnvVars(t *testing.T) {
	mgr, _ := newTestManager(t)
	for _, k := range []string{"LD_PRELOAD", "PATH", "BASH_ENV", "IFS"} {
		err := mgr.Register(&ScriptConfig{
			Name:    "x" + k,
			Command: "hello.sh",
			Env:     map[string]string{k: "evil"},
		})
		require.Error(t, err, "env var %s should be rejected", k)
	}
}

func TestRun_HappyPath(t *testing.T) {
	mgr, _ := newTestManager(t)
	require.NoError(t, mgr.Register(&ScriptConfig{
		Name:    "hello",
		Command: "hello.sh",
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := mgr.Run(ctx, "hello", "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Output, "hello")
}

func TestRun_ConcurrentBlocks(t *testing.T) {
	dir := t.TempDir()
	slow := filepath.Join(dir, "slow.sh")
	require.NoError(t, os.WriteFile(slow, []byte("#!/bin/bash\nsleep 1\n"), 0o755))

	mgr := NewManager([]string{dir}, 5, 30, 5)
	require.NoError(t, mgr.Register(&ScriptConfig{Name: "slow", Command: "slow.sh"}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	var firstErr, secondErr error
	go func() {
		defer wg.Done()
		_, firstErr = mgr.Run(ctx, "slow", "ip1")
	}()
	// Give the first one a chance to grab the lock.
	time.Sleep(100 * time.Millisecond)
	go func() {
		defer wg.Done()
		_, secondErr = mgr.Run(ctx, "slow", "ip2")
	}()
	wg.Wait()

	// Exactly one should succeed; the other should be blocked because
	// AllowConcurrent=false (default).
	successCount := 0
	if firstErr == nil {
		successCount++
	}
	if secondErr == nil {
		successCount++
	}
	assert.Equal(t, 1, successCount)
}

func TestRun_TimeoutKillsProcess(t *testing.T) {
	dir := t.TempDir()
	infinite := filepath.Join(dir, "loop.sh")
	require.NoError(t, os.WriteFile(infinite, []byte("#!/bin/bash\nwhile true; do sleep 1; done\n"), 0o755))

	// 1 second timeout
	mgr := NewManager([]string{dir}, 1, 5, 5)
	require.NoError(t, mgr.Register(&ScriptConfig{Name: "loop", Command: "loop.sh"}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	result, err := mgr.Run(ctx, "loop", "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, "timeout", result.Status)
	assert.Less(t, time.Since(start), 8*time.Second, "timeout should fire quickly")
}

func TestRun_OutputLimited(t *testing.T) {
	dir := t.TempDir()
	flood := filepath.Join(dir, "flood.sh")
	// 2 MiB of zeros
	require.NoError(t, os.WriteFile(flood, []byte("#!/bin/bash\nhead -c 2097152 /dev/zero | base64\n"), 0o755))

	mgr := NewManager([]string{dir}, 30, 60, 5)
	require.NoError(t, mgr.Register(&ScriptConfig{Name: "flood", Command: "flood.sh"}))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := mgr.Run(ctx, "flood", "127.0.0.1")
	require.NoError(t, err)
	// Output is capped at MaxOutputBytes + truncation marker.
	assert.LessOrEqual(t, len(result.Output), MaxOutputBytes+128)
}

func TestStream_NoRaceOnClose(t *testing.T) {
	dir := t.TempDir()
	noisy := filepath.Join(dir, "noisy.sh")
	// Write to BOTH stdout and stderr, then exit. Previously this could
	// trigger "send on closed channel" because stderr goroutine survived
	// stdout's close. With the WaitGroup fix it must not.
	require.NoError(t, os.WriteFile(noisy, []byte(`#!/bin/bash
for i in 1 2 3; do
    echo "out $i"
    echo "err $i" >&2
done
exit 0
`), 0o755))

	mgr := NewManager([]string{dir}, 5, 30, 5)
	require.NoError(t, mgr.Register(&ScriptConfig{Name: "noisy", Command: "noisy.sh"}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lines, err := mgr.Stream(ctx, "noisy", "127.0.0.1")
	require.NoError(t, err)

	var collected []string
	for line := range lines {
		collected = append(collected, strings.TrimRight(line, "\n"))
	}
	// We should get both stdout and stderr lines.
	joined := strings.Join(collected, "|")
	assert.Contains(t, joined, "out 1")
	assert.Contains(t, joined, "err 1")
}

func TestReplaceAll_HotReload(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.sh"), []byte("#!/bin/bash\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.sh"), []byte("#!/bin/bash\n"), 0o755))

	mgr := NewManager([]string{dir}, 5, 30, 5)
	require.NoError(t, mgr.Register(&ScriptConfig{Name: "a", Command: "a.sh"}))
	assert.True(t, mgr.Has("a"))
	assert.False(t, mgr.Has("b"))

	errs := mgr.ReplaceAll([]*ScriptConfig{
		{Name: "b", Command: "b.sh"},
	})
	assert.Empty(t, errs)
	assert.False(t, mgr.Has("a"))
	assert.True(t, mgr.Has("b"))
}

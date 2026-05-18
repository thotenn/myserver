package scripts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// MaxOutputBytes caps the total output captured/streamed per execution. A
// script that writes more than this will be truncated and the execution
// marked with a truncation notice.
const MaxOutputBytes = 1 << 20 // 1 MiB

// baseEnv returns the minimal base environment variables that are safe to
// pass to every script execution. It is computed once and reused.
func baseEnv() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"USER=myserver",
		"SHELL=/bin/bash",
	}
}

// Executor runs scripts using os/exec.
type Executor struct{}

// NewExecutor creates a new script executor.
func NewExecutor() *Executor {
	return &Executor{}
}

// buildCommand returns an *exec.Cmd set up with process group semantics so
// that children spawned by the script die with the bash process when ctx is
// cancelled. It also scrubs the environment to a minimal safe set.
func buildCommand(ctx context.Context, cfg *ScriptConfig) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/bin/bash", cfg.resolvedPath)
	if len(cfg.Args) > 0 {
		cmd.Args = append(cmd.Args, cfg.Args...)
	}

	// Minimal environment. We deliberately do NOT inherit HOMEPAGE_VAR_*,
	// API keys, or any other secret from the parent process.
	cmd.Env = append(baseEnv(), "HOME=/tmp")
	if tz := os.Getenv("TZ"); tz != "" {
		cmd.Env = append(cmd.Env, "TZ="+tz)
	}
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Put the process into its own group so we can signal the whole tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// exec.CommandContext-Kill kills the whole group instead of just bash.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Send SIGTERM to the entire process group first...
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		return nil
	}
	// ...then escalate to SIGKILL after a short grace period.
	cmd.WaitDelay = 5 * time.Second

	return cmd
}

// Execute runs a script and returns its execution result.
// The context controls the timeout and client-cancellation.
func (e *Executor) Execute(ctx context.Context, cfg *ScriptConfig, clientIP string) *Execution {
	ex := &Execution{
		Name:      cfg.Name,
		Status:    "running",
		StartedAt: time.Now(),
		ClientIP:  clientIP,
	}

	cmd := buildCommand(ctx, cfg)

	var combined limitedBuffer
	combined.max = MaxOutputBytes
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	err := cmd.Run()
	ex.Duration = time.Since(ex.StartedAt)
	ex.Output = combined.String()
	if combined.truncated {
		ex.Output += "\n[output truncated at 1MB]"
	}

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		ex.Status = "timeout"
		ex.ExitCode = -1
	case errors.Is(ctx.Err(), context.Canceled):
		ex.Status = "cancelled"
		ex.ExitCode = -1
	case err != nil:
		ex.Status = "error"
		if exitErr, ok := err.(*exec.ExitError); ok {
			ex.ExitCode = exitErr.ExitCode()
		} else {
			ex.ExitCode = -1
		}
	default:
		ex.Status = "success"
		ex.ExitCode = 0
	}

	return ex
}

// StreamOutput runs a script and sends output lines to a channel as they are
// produced. It waits for BOTH stdout and stderr goroutines to finish before
// returning, so the caller can safely close the output channel afterwards.
func (e *Executor) StreamOutput(ctx context.Context, cfg *ScriptConfig, lines chan<- string) *Execution {
	ex := &Execution{
		Name:      cfg.Name,
		Status:    "running",
		StartedAt: time.Now(),
	}

	cmd := buildCommand(ctx, cfg)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ex.Status = "error"
		ex.Output = fmt.Sprintf("error creating stdout pipe: %v", err)
		ex.ExitCode = -1
		return ex
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		ex.Status = "error"
		ex.Output = fmt.Sprintf("error creating stderr pipe: %v", err)
		ex.ExitCode = -1
		return ex
	}

	if err := cmd.Start(); err != nil {
		ex.Status = "error"
		ex.Output = fmt.Sprintf("error starting command: %v", err)
		ex.ExitCode = -1
		return ex
	}

	var mu sync.Mutex
	var outputBuf strings.Builder
	written := 0
	truncated := false

	writeChunk := func(chunk string) {
		mu.Lock()
		defer mu.Unlock()
		if truncated {
			return
		}
		remaining := MaxOutputBytes - written
		if remaining <= 0 {
			truncated = true
			return
		}
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
			truncated = true
		}
		outputBuf.WriteString(chunk)
		written += len(chunk)
		select {
		case lines <- chunk:
		case <-ctx.Done():
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)

	drain := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			writeChunk(scanner.Text() + "\n")
		}
	}

	go drain(stdout)
	go drain(stderr)

	// Wait for BOTH readers to finish before calling cmd.Wait so the
	// outputBuf is complete and no goroutine survives the return.
	wg.Wait()
	waitErr := cmd.Wait()

	ex.Duration = time.Since(ex.StartedAt)
	mu.Lock()
	ex.Output = outputBuf.String()
	wasTruncated := truncated
	mu.Unlock()
	if wasTruncated {
		ex.Output += "\n[output truncated at 1MB]"
	}

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		ex.Status = "timeout"
		ex.ExitCode = -1
	case errors.Is(ctx.Err(), context.Canceled):
		ex.Status = "cancelled"
		ex.ExitCode = -1
	case waitErr != nil:
		ex.Status = "error"
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			ex.ExitCode = exitErr.ExitCode()
		} else {
			ex.ExitCode = -1
		}
	default:
		ex.Status = "success"
		ex.ExitCode = 0
	}

	return ex
}

// limitedBuffer is an io.Writer that caps the total bytes written. Excess
// writes are discarded and the truncated flag is set.
type limitedBuffer struct {
	mu        sync.Mutex
	data      strings.Builder
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.max - b.data.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.data.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	b.data.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

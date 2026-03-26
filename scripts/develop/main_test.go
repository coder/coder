//go:build !windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
)

func TestLogWriter(t *testing.T) {
	t.Parallel()

	t.Run("SingleLine", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf)).Named("test")
		w := &logWriter{logger: logger}
		_, err := w.Write([]byte("hello\n"))
		require.NoError(t, err)
		out := buf.String()
		assert.Contains(t, out, "test:")
		assert.Contains(t, out, "hello")
	})

	t.Run("MultiLine", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf)).Named("x")
		w := &logWriter{logger: logger}
		_, err := w.Write([]byte("a\nb\nc\n"))
		require.NoError(t, err)
		out := buf.String()
		lines := strings.Split(strings.TrimSpace(out), "\n")
		require.Len(t, lines, 3)
		for _, line := range lines {
			assert.Contains(t, line, "x:")
		}
	})

	t.Run("PartialLine", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf)).Named("p")
		w := &logWriter{logger: logger}
		_, err := w.Write([]byte("no newline"))
		require.NoError(t, err)
		// Partial line should be buffered, not logged yet.
		assert.Empty(t, buf.String())
	})

	t.Run("PartialThenNewline", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf)).Named("p")
		w := &logWriter{logger: logger}

		_, err := w.Write([]byte("hello"))
		require.NoError(t, err)
		assert.Empty(t, buf.String())

		_, err = w.Write([]byte(" world\n"))
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "hello world")
	})

	t.Run("EmptyLinesSkipped", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf)).Named("e")
		w := &logWriter{logger: logger}
		_, err := w.Write([]byte("\n\nfoo\n\n"))
		require.NoError(t, err)
		out := buf.String()
		// Only "foo" should produce a log line.
		lines := strings.Split(strings.TrimSpace(out), "\n")
		assert.Len(t, lines, 1)
		assert.Contains(t, lines[0], "foo")
	})

	t.Run("ConcurrentWrites", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf)).Named("c")
		w := &logWriter{logger: logger}

		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 50 {
					_, _ = w.Write([]byte("x\n"))
				}
			}()
		}
		wg.Wait()

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		assert.Len(t, lines, 500)
		for _, line := range lines {
			assert.Contains(t, line, "c:")
			assert.Contains(t, line, "x")
		}
	})
}

func TestFilterEnv(t *testing.T) {
	t.Parallel()

	env := []string{
		"CODER_SESSION_TOKEN=secret",
		"CODER_URL=https://example.com",
		"KEEP_ME=yes",
		"PATH=/usr/bin",
	}
	result := filterEnv(env, "CODER_SESSION_TOKEN", "CODER_URL")

	for _, e := range result {
		k, _, _ := strings.Cut(e, "=")
		assert.NotEqual(t, "CODER_SESSION_TOKEN", k)
		assert.NotEqual(t, "CODER_URL", k)
	}
	assert.Contains(t, result, "KEEP_ME=yes")
	assert.Contains(t, result, "PATH=/usr/bin")
}

func TestShellBool(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "1", shellBool(true))
	assert.Equal(t, "0", shellBool(false))
}

func TestDevelopInCoder(t *testing.T) {
	t.Run("DEVELOP_IN_CODER", func(t *testing.T) {
		t.Setenv("DEVELOP_IN_CODER", "1")
		t.Setenv("CODER_AGENT_URL", "")
		assert.True(t, developInCoder())
	})

	t.Run("CODER_AGENT_URL", func(t *testing.T) {
		t.Setenv("DEVELOP_IN_CODER", "")
		t.Setenv("CODER_AGENT_URL", "http://something")
		assert.True(t, developInCoder())
	})

	t.Run("Neither", func(t *testing.T) {
		t.Setenv("DEVELOP_IN_CODER", "")
		t.Setenv("CODER_AGENT_URL", "")
		assert.False(t, developInCoder())
	})
}

func TestDevConfigValidate(t *testing.T) {
	t.Parallel()

	base := func() *devConfig {
		return &devConfig{
			apiPort:   3000,
			webPort:   8080,
			proxyPort: 3010,
			password:  defaultPassword,
		}
	}

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, base().validate())
	})

	t.Run("AgplAndProxy", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.agpl = true
		cfg.useProxy = true
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--agpl and --use-proxy")
	})

	t.Run("AgplAndMultiOrg", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.agpl = true
		cfg.multiOrg = true
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--agpl and --multi-organization")
	})

	t.Run("PortTooLow", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.apiPort = 0
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--port must be between 1 and 65535")
	})

	t.Run("PortTooHigh", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.apiPort = 70000
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--port must be between 1 and 65535")
	})

	t.Run("PortConflictWithWeb", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.apiPort = 8080
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with frontend dev server")
	})

	t.Run("PortConflictWithProxy", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.apiPort = 3010
		cfg.useProxy = true
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with workspace proxy")
	})

	t.Run("ProxyPortOKWithoutFlag", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.apiPort = 3010
		assert.NoError(t, cfg.validate())
	})

	t.Run("WebPortTooLow", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.webPort = 0
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--web-port must be between 1 and 65535")
	})

	t.Run("ProxyPortTooHigh", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.proxyPort = 70000
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--proxy-port must be between 1 and 65535")
	})

	t.Run("WebProxyPortConflict", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.webPort = 9000
		cfg.proxyPort = 9000
		cfg.useProxy = true
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--web-port 9000 conflicts with --proxy-port")
	})

	t.Run("WebProxyPortConflictOKWithoutProxy", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.webPort = 9000
		cfg.proxyPort = 9000
		assert.NoError(t, cfg.validate())
	})
}

func TestDevConfigResolveEnv(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "leaked")
	t.Setenv("CODER_URL", "https://leaked.example.com")

	cfg := &devConfig{apiPort: 3000, accessURL: defaultAccessURL}
	require.NoError(t, cfg.resolveEnv())

	wd, _ := os.Getwd()
	assert.Equal(t, wd, cfg.projectRoot)
	assert.Equal(t, filepath.Join(wd, "build",
		fmt.Sprintf("coder_%s_%s", runtime.GOOS, runtime.GOARCH)), cfg.binaryPath)
	assert.Equal(t, filepath.Join(wd, ".coderv2"), cfg.configDir)
	assert.Equal(t, "http://127.0.0.1:3000", cfg.accessURL)

	// Should have unset leaked env vars.
	assert.Empty(t, os.Getenv("CODER_SESSION_TOKEN"))
	assert.Empty(t, os.Getenv("CODER_URL"))

	// childEnv should be populated and exclude leaked vars.
	require.NotEmpty(t, cfg.childEnv)
	for _, e := range cfg.childEnv {
		k, _, _ := strings.Cut(e, "=")
		assert.NotEqual(t, "CODER_SESSION_TOKEN", k)
		assert.NotEqual(t, "CODER_URL", k)
	}
}

func TestDevConfigResolveEnvExplicitAccessURL(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "")
	t.Setenv("CODER_URL", "")

	cfg := &devConfig{apiPort: 5000, accessURL: "http://myhost:5000"}
	require.NoError(t, cfg.resolveEnv())
	assert.Equal(t, "http://myhost:5000", cfg.accessURL)
}

func TestDevConfigCmd(t *testing.T) {
	t.Parallel()

	cfg := &devConfig{
		projectRoot: "/fake/root",
		childEnv:    []string{"A=1", "B=2"},
	}

	cmd := cfg.cmd(context.Background(), "echo", "hello")
	assert.Equal(t, "/fake/root", cmd.Dir)
	assert.Equal(t, []string{"A=1", "B=2"}, cmd.Env)

	// Verify childEnv is cloned, not shared.
	cmd.Env = append(cmd.Env, "C=3")
	assert.Len(t, cfg.childEnv, 2, "original childEnv must not be mutated")
}

func TestProcGroupProcessExit(t *testing.T) {
	t.Parallel()

	logger := slog.Make(sloghuman.Sink(&bytes.Buffer{}))
	group := newProcGroup(t.Context(), logger)

	cmd := exec.CommandContext(t.Context(), "false")
	cmd.Env = os.Environ()
	require.NoError(t, group.Start("dies-fast", cmd))

	// Process exit should cancel the group context.
	select {
	case <-group.Ctx().Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for context cancellation")
	}

	// Wait should return an error naming the exited process.
	err := group.Wait()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dies-fast")
}

func TestProcGroupGracefulShutdown(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	logger := slog.Make(sloghuman.Sink(&bytes.Buffer{}))
	group := newProcGroup(ctx, logger)

	// Start a process that runs until signaled.
	cmd := exec.CommandContext(ctx, "sleep", "60")
	cmd.Env = os.Environ()
	err := group.Start("sleeper", cmd)
	require.NoError(t, err)

	// Cancel the parent context. cmd.Cancel sends SIGINT, and
	// cmd.WaitDelay escalates to SIGKILL if needed.
	cancel()

	done := make(chan error, 1)
	go func() { done <- group.Wait() }()

	select {
	case err := <-done:
		// The process was killed, so we expect an error.
		require.Error(t, err)
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("timed out waiting for graceful shutdown")
	}
}

func TestPoll(t *testing.T) {
	t.Parallel()

	t.Run("ImmediateSuccess", func(t *testing.T) {
		t.Parallel()
		val, err := poll(t.Context(), 10*time.Millisecond,
			func(_ context.Context) (string, bool, error) {
				return "done", true, nil
			})
		require.NoError(t, err)
		assert.Equal(t, "done", val)
	})

	t.Run("EventualSuccess", func(t *testing.T) {
		t.Parallel()
		calls := 0
		val, err := poll(t.Context(), 10*time.Millisecond,
			func(_ context.Context) (int, bool, error) {
				calls++
				if calls >= 3 {
					return calls, true, nil
				}
				return 0, false, nil
			})
		require.NoError(t, err)
		assert.Equal(t, 3, val)
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := poll(ctx, 10*time.Millisecond,
			func(_ context.Context) (struct{}, bool, error) {
				t.Fatal("cond should not be called")
				return struct{}{}, false, nil
			})
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("ErrorStopsPolling", func(t *testing.T) {
		t.Parallel()
		calls := 0
		_, err := poll(t.Context(), 10*time.Millisecond,
			func(_ context.Context) (string, bool, error) {
				calls++
				if calls == 2 {
					return "", false, xerrors.New("boom")
				}
				return "", false, nil
			})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		assert.Equal(t, 2, calls)
	})
}

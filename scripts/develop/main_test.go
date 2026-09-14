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
			wg.Go(func() {
				for range 50 {
					_, _ = w.Write([]byte("x\n"))
				}
			})
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

func TestPortOffset(t *testing.T) {
	t.Parallel()

	root := "/tmp/coder/worktree-a"
	offset := portOffset(root)
	assert.Equal(t, offset, portOffset(root))
	assert.GreaterOrEqual(t, offset, 0)
	assert.Less(t, offset, 1000)
	assert.Equal(t, 0, offset%10)

	var foundDifferent bool
	for _, otherRoot := range []string{
		"/tmp/coder/worktree-b",
		"/tmp/coder/worktree-c",
		"/tmp/coder/worktree-d",
	} {
		if portOffset(otherRoot) != offset {
			foundDifferent = true
			break
		}
	}
	assert.True(t, foundDifferent, "expected typical worktree paths to use different offsets")
}

func TestApplyPortOffsetSkipsExplicitPorts(t *testing.T) {
	t.Parallel()

	projectRoot := "/tmp/coder/worktree-offset"
	for i := range 100 {
		candidate := fmt.Sprintf("/tmp/coder/worktree-offset-%d", i)
		if portOffset(candidate) != 0 {
			projectRoot = candidate
			break
		}
	}
	offset := portOffset(projectRoot)
	require.NotZero(t, offset)

	cfg := &devConfig{
		apiPort:           3000,
		webPort:           8080,
		proxyPort:         3010,
		coderMetricsPort:  2114,
		portOffsetEnabled: true,
		projectRoot:       projectRoot,
		portExplicit: portExplicit{
			web:     true,
			metrics: true,
		},
	}
	cfg.applyPortOffset()

	assert.Equal(t, int64(3000+offset), cfg.apiPort)
	assert.Equal(t, int64(8080), cfg.webPort)
	assert.Equal(t, int64(3010+offset), cfg.proxyPort)
	assert.Equal(t, int64(2114), cfg.coderMetricsPort)
	assert.Equal(t, portSourceOffset, cfg.apiPortSource)
	assert.Equal(t, portSourceExplicit, cfg.webPortSource)
	assert.Equal(t, portSourceOffset, cfg.proxyPortSource)
	assert.Equal(t, portSourceExplicit, cfg.metricsPortSource)
}

func TestApplyPortOffsetDisabledUsesDefaultPorts(t *testing.T) {
	t.Parallel()

	projectRoot := "/tmp/coder/worktree-offset"
	for i := range 100 {
		candidate := fmt.Sprintf("/tmp/coder/worktree-offset-disabled-%d", i)
		if portOffset(candidate) != 0 {
			projectRoot = candidate
			break
		}
	}
	require.NotZero(t, portOffset(projectRoot))

	cfg := &devConfig{
		apiPort:          3000,
		webPort:          8080,
		proxyPort:        3010,
		coderMetricsPort: 2114,
		projectRoot:      projectRoot,
	}
	cfg.applyPortOffset()

	assert.Equal(t, int64(3000), cfg.apiPort)
	assert.Equal(t, int64(8080), cfg.webPort)
	assert.Equal(t, int64(3010), cfg.proxyPort)
	assert.Equal(t, int64(2114), cfg.coderMetricsPort)
	assert.Zero(t, cfg.portOffset)
	assert.Empty(t, cfg.apiPortSource)
	assert.Empty(t, cfg.webPortSource)
	assert.Empty(t, cfg.proxyPortSource)
	assert.Empty(t, cfg.metricsPortSource)
	assert.Equal(t, "API: 3000", portBannerLine("API", cfg.apiPort, cfg.apiPortSource, cfg.portOffset))
}

func TestPortOffsetDefaultPortsDoNotOverlap(t *testing.T) {
	t.Parallel()

	ports := []struct {
		name string
		base int
	}{
		{name: "API", base: 3000},
		{name: "Web UI", base: 8080},
		{name: "Proxy", base: 3010},
		{name: "Coder metrics", base: 2114},
	}
	seen := make(map[int]string)
	for bucket := range portOffsetBuckets {
		offset := bucket * portOffsetStep
		for _, port := range ports {
			effective := port.base + offset
			if other, ok := seen[effective]; ok {
				t.Fatalf("%s collides with %s on port %d", port.name, other, effective)
			}
			seen[effective] = fmt.Sprintf("%s with offset %d", port.name, offset)
		}
	}
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
			apiPort:          3000,
			webPort:          8080,
			proxyPort:        3010,
			coderMetricsPort: 2114,
			password:         defaultPassword,
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

	t.Run("PrometheusPortConflictWithAPI", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = 3000
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--prometheus-port 3000 conflicts with")
	})

	t.Run("PrometheusPortConflictWithWeb", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = 8080
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--prometheus-port 8080 conflicts with")
	})

	t.Run("PrometheusPortConflictWithProxy", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = 3010
		cfg.useProxy = true
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--prometheus-port 3010 conflicts with")
	})

	t.Run("PrometheusPortZeroDisabled", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = 0
		assert.NoError(t, cfg.validate())
	})

	t.Run("PrometheusPortValid", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = 9090
		assert.NoError(t, cfg.validate())
	})

	t.Run("PrometheusPortTooHigh", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = 70000
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--prometheus-port must be 0 (disabled) or between 1 and 65535")
	})

	t.Run("PrometheusPortNegative", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = -1
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--prometheus-port must be 0 (disabled) or between 1 and 65535")
	})

	t.Run("PrometheusProxyProxyConflictIgnoredWithoutProxy", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.coderMetricsPort = 3010
		assert.NoError(t, cfg.validate())
	})

	t.Run("PrometheusServerRequiresMetrics", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.prometheusServer = true
		cfg.coderMetricsPort = 0
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--prometheus-server requires prometheus to be enabled")
	})

	t.Run("PrometheusServerValid", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.prometheusServer = true
		cfg.coderMetricsPort = 2114
		assert.NoError(t, cfg.validate())
	})

	t.Run("PrometheusServerPortConflictWithAPI", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.prometheusServer = true
		cfg.apiPort = prometheusServerPort
		cfg.coderMetricsPort = 2114
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--port")
		assert.Contains(t, err.Error(), "conflicts with prometheus server")
	})

	t.Run("PrometheusServerPortConflictWithWeb", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.prometheusServer = true
		cfg.webPort = prometheusServerPort
		cfg.coderMetricsPort = 2114
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--web-port")
		assert.Contains(t, err.Error(), "conflicts with prometheus server")
	})

	t.Run("PrometheusServerPortConflictWithProxy", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.prometheusServer = true
		cfg.useProxy = true
		cfg.proxyPort = prometheusServerPort
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--proxy-port")
		assert.Contains(t, err.Error(), "conflicts with prometheus server")
	})

	t.Run("PrometheusServerPortNoProxyConflictWithoutFlag", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.prometheusServer = true
		cfg.proxyPort = prometheusServerPort
		// useProxy is false, so no conflict.
		assert.NoError(t, cfg.validate())
	})

	t.Run("PrometheusServerPortConflictWithMetrics", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.prometheusServer = true
		cfg.coderMetricsPort = prometheusServerPort
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--prometheus-port")
		assert.Contains(t, err.Error(), "conflicts with prometheus server")
	})
}

func TestDevConfigResolveEnv(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "leaked")
	t.Setenv("CODER_URL", "https://leaked.example.com")

	wd, _ := os.Getwd()
	cfg := &devConfig{apiPort: 3000, accessURL: defaultAccessURL}
	require.NoError(t, cfg.resolveEnv())

	assert.Equal(t, wd, cfg.projectRoot)
	assert.Equal(t, filepath.Join(wd, "build",
		fmt.Sprintf("coder_%s_%s", runtime.GOOS, runtime.GOARCH)), cfg.binaryPath)
	assert.Equal(t, filepath.Join(wd, ".coderv2"), cfg.configDir)
	assert.Equal(t, "http://127.0.0.1:3000", cfg.accessURL)
	assert.Equal(t, int64(3000), cfg.apiPort)
	assert.Zero(t, cfg.portOffset)

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

func TestDevConfigResolveEnvUsesDefaultPortsWithoutPortOffset(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "")
	t.Setenv("CODER_URL", "")

	baseRoot := t.TempDir()
	projectRoot := filepath.Join(baseRoot, "worktree")
	for i := range 100 {
		candidate := filepath.Join(baseRoot, fmt.Sprintf("worktree-default-%d", i))
		if portOffset(candidate) != 0 {
			projectRoot = candidate
			break
		}
	}
	require.NotZero(t, portOffset(projectRoot))
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	t.Chdir(projectRoot)

	cfg := &devConfig{
		apiPort:          3000,
		webPort:          8080,
		proxyPort:        3010,
		coderMetricsPort: 2114,
		accessURL:        defaultAccessURL,
	}
	require.NoError(t, cfg.resolveEnv())

	assert.Equal(t, projectRoot, cfg.projectRoot)
	assert.Equal(t, int64(3000), cfg.apiPort)
	assert.Equal(t, int64(8080), cfg.webPort)
	assert.Equal(t, int64(3010), cfg.proxyPort)
	assert.Equal(t, int64(2114), cfg.coderMetricsPort)
	assert.Zero(t, cfg.portOffset)
	assert.Empty(t, cfg.apiPortSource)
	assert.Empty(t, cfg.webPortSource)
	assert.Empty(t, cfg.proxyPortSource)
	assert.Empty(t, cfg.metricsPortSource)
	assert.Equal(t, "http://127.0.0.1:3000", cfg.accessURL)
}

func TestDevConfigResolveEnvAppliesPortOffsetWhenEnabled(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "")
	t.Setenv("CODER_URL", "")

	baseRoot := t.TempDir()
	projectRoot := filepath.Join(baseRoot, "worktree")
	for i := range 100 {
		candidate := filepath.Join(baseRoot, fmt.Sprintf("worktree-%d", i))
		if portOffset(candidate) != 0 {
			projectRoot = candidate
			break
		}
	}
	require.NotZero(t, portOffset(projectRoot))
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	t.Chdir(projectRoot)

	cfg := &devConfig{
		apiPort:           3000,
		webPort:           8080,
		proxyPort:         3010,
		coderMetricsPort:  2114,
		portOffsetEnabled: true,
		accessURL:         defaultAccessURL,
	}
	require.NoError(t, cfg.resolveEnv())

	offset := portOffset(projectRoot)
	assert.Equal(t, projectRoot, cfg.projectRoot)
	assert.Equal(t, int64(3000+offset), cfg.apiPort)
	assert.Equal(t, int64(8080+offset), cfg.webPort)
	assert.Equal(t, int64(3010+offset), cfg.proxyPort)
	assert.Equal(t, int64(2114+offset), cfg.coderMetricsPort)
	assert.Equal(t, offset, cfg.portOffset)
	assert.Equal(t, portSourceOffset, cfg.apiPortSource)
	assert.Equal(t, fmt.Sprintf("http://127.0.0.1:%d", 3000+offset), cfg.accessURL)
}

func TestDevConfigResolveEnvExplicitAccessURL(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "")
	t.Setenv("CODER_URL", "")

	cfg := &devConfig{
		apiPort:      5000,
		accessURL:    "http://myhost:5000",
		portExplicit: portExplicit{api: true},
	}
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

func TestStartPrometheusServerDockerMissing(t *testing.T) {
	// Not t.Parallel(): mutates PATH via t.Setenv.
	t.Setenv("PATH", "")

	logger := slog.Make(sloghuman.Sink(&bytes.Buffer{}))

	cfg := &devConfig{prometheusServer: true, coderMetricsPort: 2114}

	started, err := startPrometheusServer(t.Context(), logger, cfg)
	require.NoError(t, err)
	assert.False(t, started)
}

func TestPrometheusBannerEntry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		cfg       *devConfig
		started   bool
		wantLabel string
		wantPort  int64
	}{
		{
			name:      "MetricsDisabled",
			cfg:       &devConfig{coderMetricsPort: 0},
			started:   false,
			wantLabel: "",
			wantPort:  0,
		},
		{
			name:      "MetricsOnlyDefault",
			cfg:       &devConfig{coderMetricsPort: 2114},
			started:   false,
			wantLabel: "Metrics:",
			wantPort:  2114,
		},
		{
			name:      "PrometheusServerUp",
			cfg:       &devConfig{coderMetricsPort: 2114, prometheusServer: true},
			started:   true,
			wantLabel: "Prometheus UI:",
			wantPort:  prometheusServerPort,
		},
		{
			name:      "ServerRequestedButDown",
			cfg:       &devConfig{coderMetricsPort: 2114, prometheusServer: true},
			started:   false,
			wantLabel: "Metrics:",
			wantPort:  2114,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			label, port := prometheusBannerEntry(tc.cfg, tc.started)
			assert.Equal(t, tc.wantLabel, label)
			assert.Equal(t, tc.wantPort, port)
		})
	}
}

//nolint:paralleltest // loadEnvFile mutates process-global environment.
func TestLoadEnvFile(t *testing.T) {
	t.Run("LoadsVariablesFromFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		envFile := filepath.Join(tmpDir, ".env")
		err := os.WriteFile(envFile, []byte(strings.Join([]string{
			"# Comment line",
			"",
			"FOO_TEST_VAR=bar",
			"export BAZ_TEST_VAR=qux",
			`QUOTED_TEST_VAR="hello world"`,
			"SINGLE_QUOTED_TEST_VAR='single quoted'",
		}, "\n")), 0o600)
		require.NoError(t, err)

		// Ensure none are set beforehand.
		t.Setenv("FOO_TEST_VAR", "")
		os.Unsetenv("FOO_TEST_VAR")
		t.Setenv("BAZ_TEST_VAR", "")
		os.Unsetenv("BAZ_TEST_VAR")
		t.Setenv("QUOTED_TEST_VAR", "")
		os.Unsetenv("QUOTED_TEST_VAR")
		t.Setenv("SINGLE_QUOTED_TEST_VAR", "")
		os.Unsetenv("SINGLE_QUOTED_TEST_VAR")

		n, err := loadEnvFile(envFile)
		require.NoError(t, err)
		assert.Equal(t, 4, n)
		assert.Equal(t, "bar", os.Getenv("FOO_TEST_VAR"))
		assert.Equal(t, "qux", os.Getenv("BAZ_TEST_VAR"))
		assert.Equal(t, "hello world", os.Getenv("QUOTED_TEST_VAR"))
		assert.Equal(t, "single quoted", os.Getenv("SINGLE_QUOTED_TEST_VAR"))
	})

	t.Run("DoesNotOverrideExisting", func(t *testing.T) {
		tmpDir := t.TempDir()
		envFile := filepath.Join(tmpDir, ".env")
		err := os.WriteFile(envFile, []byte("EXISTING_TEST_VAR=new\n"), 0o600)
		require.NoError(t, err)

		t.Setenv("EXISTING_TEST_VAR", "original")

		n, err := loadEnvFile(envFile)
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, "original", os.Getenv("EXISTING_TEST_VAR"))
	})

	t.Run("ErrorsOnMissingFile", func(t *testing.T) {
		_, err := loadEnvFile("/nonexistent/path/.env")
		require.Error(t, err)
	})

	t.Run("ErrorsOnEmptyPath", func(t *testing.T) {
		// This tests the caller logic (main), but we verify loadEnvFile
		// would error on empty path since godotenv.Read("") fails.
		_, err := loadEnvFile("")
		require.Error(t, err)
	})
}

//nolint:paralleltest // parseEnvFileFlag mutates process-global os.Args.
func TestParseEnvFileFlag(t *testing.T) {
	t.Run("FlagWithSpace", func(t *testing.T) {
		orig := os.Args
		t.Cleanup(func() { os.Args = orig })
		os.Args = []string{"develop", "--env-file", "/tmp/test.env", "--port", "3000"}

		result, err := parseEnvFileFlag()
		require.NoError(t, err)
		assert.Equal(t, "/tmp/test.env", result)
	})

	t.Run("FlagWithEquals", func(t *testing.T) {
		orig := os.Args
		t.Cleanup(func() { os.Args = orig })
		os.Args = []string{"develop", "--env-file=/tmp/test.env", "--port", "3000"}

		result, err := parseEnvFileFlag()
		require.NoError(t, err)
		assert.Equal(t, "/tmp/test.env", result)
	})

	t.Run("FallsBackToEnvVar", func(t *testing.T) {
		orig := os.Args
		t.Cleanup(func() { os.Args = orig })
		os.Args = []string{"develop", "--port", "3000"}

		t.Setenv("CODER_DEV_ENV_FILE", "/tmp/from-env.env")

		result, err := parseEnvFileFlag()
		require.NoError(t, err)
		assert.Equal(t, "/tmp/from-env.env", result)
	})

	t.Run("FlagTakesPrecedenceOverEnvVar", func(t *testing.T) {
		orig := os.Args
		t.Cleanup(func() { os.Args = orig })
		os.Args = []string{"develop", "--env-file", "/tmp/from-flag.env"}

		t.Setenv("CODER_DEV_ENV_FILE", "/tmp/from-env.env")

		result, err := parseEnvFileFlag()
		require.NoError(t, err)
		assert.Equal(t, "/tmp/from-flag.env", result)
	})

	t.Run("ReturnsEmptyWhenUnset", func(t *testing.T) {
		orig := os.Args
		t.Cleanup(func() { os.Args = orig })
		os.Args = []string{"develop", "--port", "3000"}

		t.Setenv("CODER_DEV_ENV_FILE", "")
		os.Unsetenv("CODER_DEV_ENV_FILE")

		result, err := parseEnvFileFlag()
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("ErrorsWhenValueMissing", func(t *testing.T) {
		orig := os.Args
		t.Cleanup(func() { os.Args = orig })
		os.Args = []string{"develop", "--env-file"}

		_, err := parseEnvFileFlag()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--env-file requires a value")
	})
}

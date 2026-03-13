package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	t.Run("ReturnsFullLength", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf))
		w := &logWriter{logger: logger}
		input := []byte("a\nb\n")
		n, err := w.Write(input)
		require.NoError(t, err)
		assert.Equal(t, len(input), n)
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

func TestAppendEnv(t *testing.T) {
	t.Parallel()

	t.Run("Single", func(t *testing.T) {
		t.Parallel()
		env := appendEnv(nil, "K", "V")
		assert.Equal(t, []string{"K=V"}, env)
	})

	t.Run("Multiple", func(t *testing.T) {
		t.Parallel()
		env := appendEnv([]string{"EXISTING=1"}, "A", "1", "B", "2")
		assert.Equal(t, []string{"EXISTING=1", "A=1", "B=2"}, env)
	})

	t.Run("OddArgIgnored", func(t *testing.T) {
		t.Parallel()
		env := appendEnv(nil, "A", "1", "ORPHAN")
		assert.Equal(t, []string{"A=1"}, env)
	})
}

func TestCleanChildEnv(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "secret")
	t.Setenv("CODER_URL", "https://example.com")
	t.Setenv("CODER_DEV_TEST_KEEP", "yes")

	result := cleanChildEnv()

	for _, e := range result {
		k, _, _ := strings.Cut(e, "=")
		assert.NotEqual(t, "CODER_SESSION_TOKEN", k, "CODER_SESSION_TOKEN should be filtered")
		assert.NotEqual(t, "CODER_URL", k, "CODER_URL should be filtered")
	}

	found := false
	for _, e := range result {
		if strings.HasPrefix(e, "CODER_DEV_TEST_KEEP=") {
			found = true
			break
		}
	}
	assert.True(t, found, "CODER_DEV_TEST_KEEP should be preserved")
}

func TestBoolStr(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "1", boolStr(true))
	assert.Equal(t, "0", boolStr(false))
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
			apiPort:  defaultAPIPort,
			password: defaultPassword,
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
		cfg.apiPort = webPort
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with frontend dev server")
	})

	t.Run("PortConflictWithProxy", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.apiPort = proxyPort
		cfg.useProxy = true
		err := cfg.validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with workspace proxy")
	})

	t.Run("ProxyPortOKWithoutFlag", func(t *testing.T) {
		t.Parallel()
		cfg := base()
		cfg.apiPort = proxyPort
		assert.NoError(t, cfg.validate())
	})
}

func TestDevConfigResolvePaths(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "leaked")
	t.Setenv("CODER_URL", "https://leaked.example.com")

	cfg := &devConfig{apiPort: defaultAPIPort}
	require.NoError(t, cfg.resolvePaths())

	wd, _ := os.Getwd()
	assert.Equal(t, wd, cfg.projectRoot)
	assert.Equal(t, filepath.Join(wd, "build",
		fmt.Sprintf("coder_%s_%s", runtime.GOOS, runtime.GOARCH)), cfg.binaryPath)
	assert.Equal(t, filepath.Join(wd, ".coderv2"), cfg.configDir)

	// Should have unset leaked env vars.
	assert.Empty(t, os.Getenv("CODER_SESSION_TOKEN"))
	assert.Empty(t, os.Getenv("CODER_URL"))
}

func TestDevConfigResolvePathsDefaultAccessURL(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "")
	t.Setenv("CODER_URL", "")

	cfg := &devConfig{apiPort: 5000}
	require.NoError(t, cfg.resolvePaths())
	assert.Equal(t, "http://127.0.0.1:5000", cfg.accessURL)
}

func TestDevConfigResolvePathsExplicitAccessURL(t *testing.T) {
	t.Setenv("CODER_SESSION_TOKEN", "")
	t.Setenv("CODER_URL", "")

	cfg := &devConfig{apiPort: 5000, accessURL: "http://myhost:5000"}
	require.NoError(t, cfg.resolvePaths())
	assert.Equal(t, "http://myhost:5000", cfg.accessURL)
}

func TestMergeExits(t *testing.T) {
	t.Parallel()

	t.Run("FirstProcToExit", func(t *testing.T) {
		t.Parallel()

		p1 := &proc{name: "slow", done: make(chan struct{})}
		p2 := &proc{name: "fast", done: make(chan struct{})}

		ch := mergeExits([]*proc{p1, p2})
		close(p2.done)

		select {
		case got := <-ch:
			assert.Equal(t, "fast", got.name)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for exit")
		}
	})

	t.Run("EmptySlice", func(t *testing.T) {
		t.Parallel()
		ch := mergeExits(nil)
		select {
		case <-ch:
			t.Fatal("should not receive on empty slice")
		case <-time.After(50 * time.Millisecond):
			// expected
		}
	})
}

func TestShutdownProcs(t *testing.T) {
	t.Parallel()

	// Verify shutdownProcs returns immediately for an empty slice.
	logger := slog.Make(sloghuman.Sink(&bytes.Buffer{}))
	start := time.Now()
	shutdownProcs(logger, nil, 5*time.Second)
	assert.Less(t, time.Since(start), time.Second)
}

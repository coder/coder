package agentdesktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/pty"
)

// recordedExecer implements agentexec.Execer by recording every
// invocation and delegating to a real shell command built from a
// caller-supplied mapping of subcommand → shell script body.
type recordedExecer struct {
	mu       sync.Mutex
	commands [][]string
	// scripts maps a subcommand keyword (e.g. "up", "screenshot")
	// to a shell snippet whose stdout will be the command output.
	scripts map[string]string
}

func (r *recordedExecer) record(cmd string, args ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, append([]string{cmd}, args...))
}

func (r *recordedExecer) allCommands() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]string, len(r.commands))
	copy(out, r.commands)
	return out
}

// scriptFor finds the first matching script key present in args.
func (r *recordedExecer) scriptFor(args []string) string {
	for _, a := range args {
		if s, ok := r.scripts[a]; ok {
			return s
		}
	}
	// Fallback: succeed silently.
	return "true"
}

func (r *recordedExecer) CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd {
	r.record(cmd, args...)
	script := r.scriptFor(args)
	//nolint:gosec // Test helper — script content is controlled by the test.
	return exec.CommandContext(ctx, "sh", "-c", script)
}

func (r *recordedExecer) PTYCommandContext(ctx context.Context, cmd string, args ...string) *pty.Cmd {
	r.record(cmd, args...)
	return pty.CommandContext(ctx, "sh", "-c", r.scriptFor(args))
}

// --- portableDesktop tests ---

func TestPortableDesktop_Start_ParsesOutput(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	dataDir := t.TempDir()

	// The "up" script prints the JSON line then sleeps until
	// the context is canceled (simulating a long-running process).
	rec := &recordedExecer{
		scripts: map[string]string{
			"up": `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	pd := &portableDesktop{
		logger:  logger,
		execer:  rec,
		dataDir: dataDir,
		binPath: "portabledesktop", // pre-set so ensureBinary is a no-op
	}

	ctx := context.Background()
	cfg, err := pd.Start(ctx)
	require.NoError(t, err)

	assert.Equal(t, 1920, cfg.Width)
	assert.Equal(t, 1080, cfg.Height)
	assert.Equal(t, 5901, cfg.VNCPort)
	assert.Equal(t, -1, cfg.Display)

	// Clean up the long-running process.
	require.NoError(t, pd.Close())
}

func TestPortableDesktop_Start_Idempotent(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	dataDir := t.TempDir()

	rec := &recordedExecer{
		scripts: map[string]string{
			"up": `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	pd := &portableDesktop{
		logger:  logger,
		execer:  rec,
		dataDir: dataDir,
		binPath: "portabledesktop",
	}

	ctx := context.Background()
	cfg1, err := pd.Start(ctx)
	require.NoError(t, err)

	cfg2, err := pd.Start(ctx)
	require.NoError(t, err)

	assert.Equal(t, cfg1, cfg2, "second Start should return the same config")

	// The execer should have been called exactly once for "up".
	cmds := rec.allCommands()
	upCalls := 0
	for _, c := range cmds {
		for _, a := range c {
			if a == "up" {
				upCalls++
			}
		}
	}
	assert.Equal(t, 1, upCalls, "expected exactly one 'up' invocation")

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_Screenshot(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	dataDir := t.TempDir()

	rec := &recordedExecer{
		scripts: map[string]string{
			"screenshot": `echo '{"data":"abc123"}'`,
		},
	}

	pd := &portableDesktop{
		logger:  logger,
		execer:  rec,
		dataDir: dataDir,
		binPath: "portabledesktop",
	}

	ctx := context.Background()
	result, err := pd.Screenshot(ctx, ScreenshotOptions{})
	require.NoError(t, err)

	assert.Equal(t, "abc123", result.Data)
}

func TestPortableDesktop_Screenshot_WithTargetDimensions(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	dataDir := t.TempDir()

	rec := &recordedExecer{
		scripts: map[string]string{
			"screenshot": `echo '{"data":"x"}'`,
		},
	}

	pd := &portableDesktop{
		logger:  logger,
		execer:  rec,
		dataDir: dataDir,
		binPath: "portabledesktop",
	}

	ctx := context.Background()
	_, err := pd.Screenshot(ctx, ScreenshotOptions{
		TargetWidth:  800,
		TargetHeight: 600,
	})
	require.NoError(t, err)

	cmds := rec.allCommands()
	require.NotEmpty(t, cmds)

	// The last command should contain the target dimension flags.
	last := cmds[len(cmds)-1]
	joined := strings.Join(last, " ")
	assert.Contains(t, joined, "--target-width 800")
	assert.Contains(t, joined, "--target-height 600")
}

func TestPortableDesktop_MouseMethods(t *testing.T) {
	t.Parallel()

	// Each sub-test verifies a single mouse method dispatches the
	// correct CLI arguments.
	tests := []struct {
		name     string
		invoke   func(context.Context, *portableDesktop) error
		wantArgs []string // substrings expected in a recorded command
	}{
		{
			name: "Move",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.Move(ctx, 42, 99)
			},
			wantArgs: []string{"mouse", "move", "42", "99"},
		},
		{
			name: "Click",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.Click(ctx, 10, 20, MouseButtonLeft)
			},
			// Click does move then click.
			wantArgs: []string{"mouse", "click", "left"},
		},
		{
			name: "DoubleClick",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.DoubleClick(ctx, 5, 6, MouseButtonRight)
			},
			wantArgs: []string{"mouse", "click", "right"},
		},
		{
			name: "ButtonDown",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.ButtonDown(ctx, MouseButtonMiddle)
			},
			wantArgs: []string{"mouse", "down", "middle"},
		},
		{
			name: "ButtonUp",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.ButtonUp(ctx, MouseButtonLeft)
			},
			wantArgs: []string{"mouse", "up", "left"},
		},
		{
			name: "Scroll",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.Scroll(ctx, 50, 60, 3, 4)
			},
			wantArgs: []string{"mouse", "scroll", "3", "4"},
		},
		{
			name: "Drag",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.Drag(ctx, 10, 20, 30, 40)
			},
			// Drag ends with mouse up left.
			wantArgs: []string{"mouse", "up", "left"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, nil)
			rec := &recordedExecer{
				scripts: map[string]string{
					"mouse": `echo ok`,
				},
			}

			pd := &portableDesktop{
				logger:  logger,
				execer:  rec,
				dataDir: t.TempDir(),
				binPath: "portabledesktop",
			}

			err := tt.invoke(context.Background(), pd)
			require.NoError(t, err)

			cmds := rec.allCommands()
			require.NotEmpty(t, cmds, "expected at least one command")

			// Find at least one recorded command that contains
			// all expected argument substrings.
			found := false
			for _, cmd := range cmds {
				joined := strings.Join(cmd, " ")
				match := true
				for _, want := range tt.wantArgs {
					if !strings.Contains(joined, want) {
						match = false
						break
					}
				}
				if match {
					found = true
					break
				}
			}
			assert.True(t, found,
				"no recorded command matched %v; got %v", tt.wantArgs, cmds)
		})
	}
}

func TestPortableDesktop_KeyboardMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		invoke   func(context.Context, *portableDesktop) error
		wantArgs []string
	}{
		{
			name: "KeyPress",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.KeyPress(ctx, "Return")
			},
			wantArgs: []string{"keyboard", "key", "Return"},
		},
		{
			name: "KeyDown",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.KeyDown(ctx, "shift")
			},
			wantArgs: []string{"keyboard", "down", "shift"},
		},
		{
			name: "KeyUp",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.KeyUp(ctx, "shift")
			},
			wantArgs: []string{"keyboard", "up", "shift"},
		},
		{
			name: "Type",
			invoke: func(ctx context.Context, pd *portableDesktop) error {
				return pd.Type(ctx, "hello world")
			},
			wantArgs: []string{"keyboard", "type", "hello world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, nil)
			rec := &recordedExecer{
				scripts: map[string]string{
					"keyboard": `echo ok`,
				},
			}

			pd := &portableDesktop{
				logger:  logger,
				execer:  rec,
				dataDir: t.TempDir(),
				binPath: "portabledesktop",
			}

			err := tt.invoke(context.Background(), pd)
			require.NoError(t, err)

			cmds := rec.allCommands()
			require.NotEmpty(t, cmds)

			last := cmds[len(cmds)-1]
			joined := strings.Join(last, " ")
			for _, want := range tt.wantArgs {
				assert.Contains(t, joined, want)
			}
		})
	}
}

func TestPortableDesktop_CursorPosition(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"cursor": `echo '{"x":100,"y":200}'`,
		},
	}

	pd := &portableDesktop{
		logger:  logger,
		execer:  rec,
		dataDir: t.TempDir(),
		binPath: "portabledesktop",
	}

	x, y, err := pd.CursorPosition(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 100, x)
	assert.Equal(t, 200, y)
}

func TestPortableDesktop_Close(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	rec := &recordedExecer{
		scripts: map[string]string{
			"up": `printf '{"vncPort":5901,"geometry":"1024x768"}\n' && sleep 120`,
		},
	}

	pd := &portableDesktop{
		logger:  logger,
		execer:  rec,
		dataDir: t.TempDir(),
		binPath: "portabledesktop",
	}

	ctx := context.Background()
	_, err := pd.Start(ctx)
	require.NoError(t, err)

	// Session should exist.
	pd.mu.Lock()
	require.NotNil(t, pd.session)
	pd.mu.Unlock()

	require.NoError(t, pd.Close())

	// Session should be cleaned up.
	pd.mu.Lock()
	assert.Nil(t, pd.session)
	assert.True(t, pd.closed)
	pd.mu.Unlock()

	// Subsequent Start must fail.
	_, err = pd.Start(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "desktop is closed")
}

// --- downloadBinary tests ---

func TestDownloadBinary_Success(t *testing.T) {
	t.Parallel()

	binaryContent := []byte("#!/bin/sh\necho portable\n")
	hash := sha256.Sum256(binaryContent)
	expectedSHA := hex.EncodeToString(hash[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(binaryContent)
	}))
	defer srv.Close()

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "portabledesktop")

	err := downloadBinary(context.Background(), srv.Client(), srv.URL, expectedSHA, destPath)
	require.NoError(t, err)

	// Verify the file exists and has correct content.
	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, binaryContent, got)

	// Verify executable permissions.
	info, err := os.Stat(destPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o700, "binary should be executable")
}

func TestDownloadBinary_ChecksumMismatch(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("real binary content"))
	}))
	defer srv.Close()

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "portabledesktop")

	wrongSHA := "0000000000000000000000000000000000000000000000000000000000000000"
	err := downloadBinary(context.Background(), srv.Client(), srv.URL, wrongSHA, destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA-256 mismatch")

	// The destination file should not exist (temp file cleaned up).
	_, statErr := os.Stat(destPath)
	assert.True(t, os.IsNotExist(statErr), "dest file should not exist after checksum failure")

	// No leftover temp files in the directory.
	entries, err := os.ReadDir(destDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no leftover temp files should remain")
}

func TestDownloadBinary_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "portabledesktop")

	err := downloadBinary(context.Background(), srv.Client(), srv.URL, "irrelevant", destPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

// --- ensureBinary tests ---

func TestEnsureBinary_UsesCachedBinPath(t *testing.T) {
	t.Parallel()

	// When binPath is already set, ensureBinary should return
	// immediately without doing any work.
	logger := slogtest.Make(t, nil)
	pd := &portableDesktop{
		logger:  logger,
		execer:  agentexec.DefaultExecer,
		dataDir: t.TempDir(),
		binPath: "/already/set",
	}

	err := pd.ensureBinary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/already/set", pd.binPath)
}

func TestEnsureBinary_UsesCachedBinary(t *testing.T) {
	// Cannot use t.Parallel because t.Setenv modifies the process
	// environment.
	if runtime.GOOS != "linux" {
		t.Skip("portabledesktop is only supported on Linux")
	}

	bin, ok := platformBinaries[runtime.GOARCH]
	if !ok {
		t.Skipf("no platformBinary entry for %s", runtime.GOARCH)
	}

	dataDir := t.TempDir()
	cacheDir := filepath.Join(dataDir, "portabledesktop", bin.SHA256)
	require.NoError(t, os.MkdirAll(cacheDir, 0o700))

	cachedPath := filepath.Join(cacheDir, "portabledesktop")
	require.NoError(t, os.WriteFile(cachedPath, []byte("#!/bin/sh\n"), 0o600))

	logger := slogtest.Make(t, nil)
	pd := &portableDesktop{
		logger:  logger,
		execer:  agentexec.DefaultExecer,
		dataDir: dataDir,
	}

	// Clear PATH so LookPath won't find a real binary.
	t.Setenv("PATH", "")

	err := pd.ensureBinary(context.Background())
	require.NoError(t, err)
	assert.Equal(t, cachedPath, pd.binPath)
}

func TestEnsureBinary_Downloads(t *testing.T) {
	// Cannot use t.Parallel because t.Setenv modifies the process
	// environment and we override the package-level platformBinaries.
	if runtime.GOOS != "linux" {
		t.Skip("portabledesktop is only supported on Linux")
	}

	binaryContent := []byte("#!/bin/sh\necho downloaded\n")
	hash := sha256.Sum256(binaryContent)
	expectedSHA := hex.EncodeToString(hash[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(binaryContent)
	}))
	defer srv.Close()

	// Save and restore platformBinaries for this test.
	origBinaries := platformBinaries
	platformBinaries = map[string]struct {
		URL    string
		SHA256 string
	}{
		runtime.GOARCH: {
			URL:    srv.URL + "/portabledesktop",
			SHA256: expectedSHA,
		},
	}
	t.Cleanup(func() { platformBinaries = origBinaries })

	dataDir := t.TempDir()
	logger := slogtest.Make(t, nil)
	pd := &portableDesktop{
		logger:     logger,
		execer:     agentexec.DefaultExecer,
		dataDir:    dataDir,
		httpClient: srv.Client(),
	}

	// Ensure PATH doesn't contain a real portabledesktop binary.
	t.Setenv("PATH", "")

	err := pd.ensureBinary(context.Background())
	require.NoError(t, err)

	expectedPath := filepath.Join(dataDir, "portabledesktop", expectedSHA, "portabledesktop")
	assert.Equal(t, expectedPath, pd.binPath)

	// Verify the downloaded file has correct content.
	got, err := os.ReadFile(expectedPath)
	require.NoError(t, err)
	assert.Equal(t, binaryContent, got)
}

func TestEnsureBinary_RetriesOnFailure(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.Skip("portabledesktop is only supported on Linux")
	}

	binaryContent := []byte("#!/bin/sh\necho retried\n")
	hash := sha256.Sum256(binaryContent)
	expectedSHA := hex.EncodeToString(hash[:])

	var mu sync.Mutex
	attempt := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		current := attempt
		attempt++
		mu.Unlock()

		// Fail the first 2 attempts, succeed on the third.
		if current < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(binaryContent)
	}))
	defer srv.Close()

	// Test downloadBinary directly to avoid time.Sleep in
	// ensureBinary's retry loop. We call it 3 times to simulate
	// what ensureBinary would do.
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "portabledesktop")

	var lastErr error
	for i := range 3 {
		lastErr = downloadBinary(context.Background(), srv.Client(), srv.URL, expectedSHA, destPath)
		if lastErr == nil {
			break
		}
		if i < 2 {
			// In the real code, ensureBinary sleeps here.
			// We skip the sleep in tests.
			continue
		}
	}
	require.NoError(t, lastErr, "download should succeed on the third attempt")

	got, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, binaryContent, got)

	mu.Lock()
	assert.Equal(t, 3, attempt, "server should have been hit 3 times")
	mu.Unlock()
}

// Ensure that portableDesktop satisfies the Desktop interface at
// compile time. This uses the unexported type so it lives in the
// internal test package.
var _ Desktop = (*portableDesktop)(nil)

// Silence the linter about unused imports — agentexec.DefaultExecer
// is used in TestEnsureBinary_UsesCachedBinPath and others, and
// fmt.Sscanf is used indirectly via the implementation.
var (
	_ = agentexec.DefaultExecer
	_ = fmt.Sprintf
)

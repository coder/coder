package agentdesktop

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

	// The "up" script prints the JSON line then sleeps until
	// the context is canceled (simulating a long-running process).
	rec := &recordedExecer{
		scripts: map[string]string{
			"up": `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		binPath:      "portabledesktop", // pre-set so ensureBinary is a no-op
		clock:        quartz.NewReal(),
	}

	ctx := t.Context()
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

	rec := &recordedExecer{
		scripts: map[string]string{
			"up": `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		binPath:      "portabledesktop",
		clock:        quartz.NewReal(),
	}

	ctx := t.Context()
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

	rec := &recordedExecer{
		scripts: map[string]string{
			"screenshot": `echo '{"data":"abc123"}'`,
		},
	}

	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		binPath:      "portabledesktop",
		clock:        quartz.NewReal(),
	}

	ctx := t.Context()
	result, err := pd.Screenshot(ctx, ScreenshotOptions{})
	require.NoError(t, err)

	assert.Equal(t, "abc123", result.Data)
}

func TestPortableDesktop_Screenshot_WithTargetDimensions(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	rec := &recordedExecer{
		scripts: map[string]string{
			"screenshot": `echo '{"data":"x"}'`,
		},
	}

	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		binPath:      "portabledesktop",
		clock:        quartz.NewReal(),
	}

	ctx := t.Context()
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
				logger:       logger,
				execer:       rec,
				scriptBinDir: t.TempDir(),
				binPath:      "portabledesktop",
				clock:        quartz.NewReal(),
			}

			err := tt.invoke(t.Context(), pd)
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
				logger:       logger,
				execer:       rec,
				scriptBinDir: t.TempDir(),
				binPath:      "portabledesktop",
				clock:        quartz.NewReal(),
			}

			err := tt.invoke(t.Context(), pd)
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
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		binPath:      "portabledesktop",
	}

	x, y, err := pd.CursorPosition(t.Context())
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
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		binPath:      "portabledesktop",
		clock:        quartz.NewReal(),
	}

	ctx := t.Context()
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
	assert.Contains(t, err.Error(), "desktop closed")
}

// --- ensureBinary tests ---

func TestEnsureBinary_UsesCachedBinPath(t *testing.T) {
	t.Parallel()

	// When binPath is already set, ensureBinary should return
	// immediately without doing any work.
	logger := slogtest.Make(t, nil)
	pd := &portableDesktop{
		logger:       logger,
		execer:       agentexec.DefaultExecer,
		scriptBinDir: t.TempDir(),
		binPath:      "/already/set",
	}

	err := pd.ensureBinary(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "/already/set", pd.binPath)
}

func TestEnsureBinary_UsesScriptBinDir(t *testing.T) {
	// Cannot use t.Parallel because t.Setenv modifies the process
	// environment.

	scriptBinDir := t.TempDir()
	binPath := filepath.Join(scriptBinDir, "portabledesktop")
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o600))
	require.NoError(t, os.Chmod(binPath, 0o755))

	logger := slogtest.Make(t, nil)
	pd := &portableDesktop{
		logger:       logger,
		execer:       agentexec.DefaultExecer,
		scriptBinDir: scriptBinDir,
	}

	// Clear PATH so LookPath won't find a real binary.
	t.Setenv("PATH", "")

	err := pd.ensureBinary(t.Context())
	require.NoError(t, err)
	assert.Equal(t, binPath, pd.binPath)
}

func TestEnsureBinary_ScriptBinDirNotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix permission bits")
	}
	// Cannot use t.Parallel because t.Setenv modifies the process
	// environment.

	scriptBinDir := t.TempDir()
	binPath := filepath.Join(scriptBinDir, "portabledesktop")
	// Write without execute permission.
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o600))
	_ = binPath

	logger := slogtest.Make(t, nil)
	pd := &portableDesktop{
		logger:       logger,
		execer:       agentexec.DefaultExecer,
		scriptBinDir: scriptBinDir,
	}

	// Clear PATH so LookPath won't find a real binary.
	t.Setenv("PATH", "")

	err := pd.ensureBinary(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestEnsureBinary_NotFound(t *testing.T) {
	// Cannot use t.Parallel because t.Setenv modifies the process
	// environment.

	logger := slogtest.Make(t, nil)
	pd := &portableDesktop{
		logger:       logger,
		execer:       agentexec.DefaultExecer,
		scriptBinDir: t.TempDir(), // empty directory
	}

	// Clear PATH so LookPath won't find a real binary.
	t.Setenv("PATH", "")

	err := pd.ensureBinary(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPortableDesktop_StartRecording(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"record": `trap 'exit 0' INT; sleep 120 & wait`,
			"up":     `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewReal()
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()
	recID := uuid.New().String()
	err := pd.StartRecording(ctx, recID)
	require.NoError(t, err)

	cmds := rec.allCommands()
	require.NotEmpty(t, cmds)
	// Find the record command (not the up command).
	found := false
	for _, cmd := range cmds {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "record") && strings.Contains(joined, "coder-recording-"+recID) {
			found = true
			assert.Contains(t, joined, "--thumbnail", "record command should include --thumbnail flag")
			break
		}
	}
	assert.True(t, found, "expected a record command with the recording ID")

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_StartRecording_ConcurrentLimit(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"record": `trap 'exit 0' INT; sleep 120 & wait`,
			"up":     `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewReal()
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()

	for i := range maxConcurrentRecordings {
		err := pd.StartRecording(ctx, uuid.New().String())
		require.NoError(t, err, "recording %d should succeed", i)
	}

	err := pd.StartRecording(ctx, uuid.New().String())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many concurrent recordings")

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_StopRecording_ReturnsArtifact(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			// Use exec so SIGINT is delivered directly to sleep
			// and the process exits immediately. (See coder/internal#1462.)
			"record": `exec sleep 120`,
			"up":     `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewReal()
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()
	recID := uuid.New().String()
	err := pd.StartRecording(ctx, recID)
	require.NoError(t, err)

	// Write a dummy MP4 file at the expected path so StopRecording
	// can open it as an artifact.
	filePath := filepath.Join(os.TempDir(), "coder-recording-"+recID+".mp4")
	require.NoError(t, os.WriteFile(filePath, []byte("fake-mp4-data"), 0o600))
	t.Cleanup(func() { _ = os.Remove(filePath) })

	artifact, err := pd.StopRecording(ctx, recID)
	require.NoError(t, err)
	defer artifact.Reader.Close()
	assert.Equal(t, int64(len("fake-mp4-data")), artifact.Size)

	// No thumbnail file exists, so ThumbnailReader should be nil.
	assert.Nil(t, artifact.ThumbnailReader, "ThumbnailReader should be nil when no thumbnail file exists")

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_StopRecording_WithThumbnail(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			// See TestPortableDesktop_StopRecording_ReturnsArtifact
			// for why we use exec instead of trap+wait.
			"record": `exec sleep 120`,
			"up":     `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewReal()
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()
	recID := uuid.New().String()
	err := pd.StartRecording(ctx, recID)
	require.NoError(t, err)

	// Write a dummy MP4 file at the expected path.
	filePath := filepath.Join(os.TempDir(), "coder-recording-"+recID+".mp4")
	require.NoError(t, os.WriteFile(filePath, []byte("fake-mp4-data"), 0o600))
	t.Cleanup(func() { _ = os.Remove(filePath) })

	// Write a thumbnail file at the expected path.
	thumbPath := filepath.Join(os.TempDir(), "coder-recording-"+recID+".thumb.jpg")
	thumbContent := []byte("fake-jpeg-thumbnail")
	require.NoError(t, os.WriteFile(thumbPath, thumbContent, 0o600))
	t.Cleanup(func() { _ = os.Remove(thumbPath) })

	artifact, err := pd.StopRecording(ctx, recID)
	require.NoError(t, err)
	defer artifact.Reader.Close()

	assert.Equal(t, int64(len("fake-mp4-data")), artifact.Size)

	// Thumbnail should be attached.
	require.NotNil(t, artifact.ThumbnailReader, "ThumbnailReader should be non-nil when thumbnail file exists")
	defer artifact.ThumbnailReader.Close()
	assert.Equal(t, int64(len(thumbContent)), artifact.ThumbnailSize)

	// Read and verify thumbnail content.
	thumbData, err := io.ReadAll(artifact.ThumbnailReader)
	require.NoError(t, err)
	assert.Equal(t, thumbContent, thumbData)

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_StopRecording_UnknownID(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"record": `trap 'exit 0' INT; sleep 120 & wait`,
		},
	}

	clk := quartz.NewReal()
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()
	_, err := pd.StopRecording(ctx, uuid.New().String())
	require.ErrorIs(t, err, ErrUnknownRecording)

	require.NoError(t, pd.Close())
}

// Ensure that portableDesktop satisfies the Desktop interface at
// compile time. This uses the unexported type so it lives in the
// internal test package.
var _ Desktop = (*portableDesktop)(nil)

func TestPortableDesktop_IdleTimeout_StopsRecordings(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"record": `trap 'exit 0' INT; sleep 120 & wait`,
			"up":     `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewMock(t)
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()
	recID := uuid.New().String()

	// Install the trap before StartRecording so it is guaranteed
	// to catch the idle monitor's NewTimer call regardless of
	// goroutine scheduling.
	trap := clk.Trap().NewTimer("agentdesktop", "recording_idle")

	err := pd.StartRecording(ctx, recID)
	require.NoError(t, err)

	// Verify recording is active.
	pd.mu.Lock()
	require.False(t, pd.recordings[recID].stopped)
	pd.mu.Unlock()

	// Wait for the idle monitor timer to be created and release
	// it so the monitor enters its select loop.
	trap.MustWait(ctx).MustRelease(ctx)
	trap.Close()

	// The stop-all path calls lockedStopRecordingProcess which
	// creates a per-recording 15s stop_timeout timer.
	stopTrap := clk.Trap().NewTimer("agentdesktop", "stop_timeout")

	// Advance past idle timeout to trigger the stop-all.
	clk.Advance(idleTimeout).MustWait(ctx)

	// Wait for the stop timer to be created, then release it.
	stopTrap.MustWait(ctx).MustRelease(ctx)
	stopTrap.Close()

	// Advance past the 15s stop timeout so the process is
	// forcibly killed. Without this the test depends on the real
	// shell handling SIGINT promptly, which is unreliable on
	// macOS CI runners (the flake in #1461).
	clk.Advance(15 * time.Second).MustWait(ctx)

	// The recording process should now be stopped.
	require.Eventually(t, func() bool {
		pd.mu.Lock()
		defer pd.mu.Unlock()
		rec, ok := pd.recordings[recID]
		return ok && rec.stopped
	}, testutil.WaitShort, testutil.IntervalFast)

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_IdleTimeout_ActivityResetsTimer(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"record": `trap 'exit 0' INT; sleep 120 & wait`,
			"up":     `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewMock(t)
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()
	recID := uuid.New().String()

	// Install the trap before StartRecording so it is guaranteed
	// to catch the idle monitor's NewTimer call regardless of
	// goroutine scheduling.
	trap := clk.Trap().NewTimer("agentdesktop", "recording_idle")

	err := pd.StartRecording(ctx, recID)
	require.NoError(t, err)

	// Wait for the idle monitor timer to be created.
	trap.MustWait(ctx).MustRelease(ctx)
	trap.Close()

	// Advance most of the way but not past the timeout.
	clk.Advance(idleTimeout - time.Minute)

	// Record activity to reset the timer.
	pd.RecordActivity()

	// Trap the Reset call that the idle monitor makes when it
	// sees recent activity.
	resetTrap := clk.Trap().TimerReset("agentdesktop", "recording_idle")

	// Advance past the original idle timeout deadline. The
	// monitor should see the recent activity and reset instead
	// of stopping.
	clk.Advance(time.Minute)

	resetTrap.MustWait(ctx).MustRelease(ctx)
	resetTrap.Close()

	// Recording should still be active because activity was
	// recorded.
	pd.mu.Lock()
	require.False(t, pd.recordings[recID].stopped)
	pd.mu.Unlock()

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_IdleTimeout_MultipleRecordings(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"record": `trap 'exit 0' INT; sleep 120 & wait`,
			"up":     `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewMock(t)
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	ctx := t.Context()
	recID1 := uuid.New().String()
	recID2 := uuid.New().String()

	// Trap idle timer creation for both recordings.
	trap := clk.Trap().NewTimer("agentdesktop", "recording_idle")

	err := pd.StartRecording(ctx, recID1)
	require.NoError(t, err)

	// Wait for first recording's idle timer.
	trap.MustWait(ctx).MustRelease(ctx)

	err = pd.StartRecording(ctx, recID2)
	require.NoError(t, err)

	// Wait for second recording's idle timer.
	trap.MustWait(ctx).MustRelease(ctx)
	trap.Close()

	// Trap the stop timers that will be created when idle fires.
	stopTrap := clk.Trap().NewTimer("agentdesktop", "stop_timeout")

	// Advance past idle timeout.
	clk.Advance(idleTimeout).MustWait(ctx)

	// Each idle monitor goroutine serializes on p.mu, so the
	// second stop timer is only created after the first stop
	// completes. Advance past the 15s stop timeout after each
	// release so the process is forcibly killed instead of
	// depending on SIGINT (unreliable on macOS — see #1461).
	stopTrap.MustWait(ctx).MustRelease(ctx)
	clk.Advance(15 * time.Second).MustWait(ctx)
	stopTrap.MustWait(ctx).MustRelease(ctx)
	clk.Advance(15 * time.Second).MustWait(ctx)
	stopTrap.Close()

	// Both recordings should be stopped.
	require.Eventually(t, func() bool {
		pd.mu.Lock()
		defer pd.mu.Unlock()
		r1, ok1 := pd.recordings[recID1]
		r2, ok2 := pd.recordings[recID2]
		return ok1 && r1.stopped && ok2 && r2.stopped
	}, testutil.WaitShort, testutil.IntervalFast)

	require.NoError(t, pd.Close())
}

func TestPortableDesktop_StartRecording_ReturnsErrDesktopClosed(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"up": `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	clk := quartz.NewReal()
	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        clk,
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(clk.Now().UnixNano())

	// Start and close the desktop so it's in the closed state.
	ctx := t.Context()
	_, err := pd.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, pd.Close())

	// StartRecording should now return ErrDesktopClosed.
	err = pd.StartRecording(ctx, uuid.New().String())
	require.ErrorIs(t, err, ErrDesktopClosed)
}

func TestPortableDesktop_Start_ReturnsErrDesktopClosed(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	rec := &recordedExecer{
		scripts: map[string]string{
			"up": `printf '{"vncPort":5901,"geometry":"1920x1080"}\n' && sleep 120`,
		},
	}

	pd := &portableDesktop{
		logger:       logger,
		execer:       rec,
		scriptBinDir: t.TempDir(),
		clock:        quartz.NewReal(),
		binPath:      "portabledesktop",
		recordings:   make(map[string]*recordingProcess),
	}
	pd.lastDesktopActionAt.Store(pd.clock.Now().UnixNano())

	ctx := t.Context()
	_, err := pd.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, pd.Close())

	_, err = pd.Start(ctx)
	require.ErrorIs(t, err, ErrDesktopClosed)
}

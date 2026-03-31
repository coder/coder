package agentdesktop

import (
	"context"
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
	assert.Contains(t, err.Error(), "desktop is closed")
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

// Ensure that portableDesktop satisfies the Desktop interface at
// compile time. This uses the unexported type so it lives in the
// internal test package.
var _ Desktop = (*portableDesktop)(nil)

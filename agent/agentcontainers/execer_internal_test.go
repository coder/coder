package agentcontainers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/pty"
)

// recordingExecer captures each forwarded argv tuple so internal tests can
// assert on what commandEnvExecer hands to its inner Execer.
type recordingExecer struct {
	commands [][]string
}

func (r *recordingExecer) CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd {
	r.commands = append(r.commands, append([]string{cmd}, args...))
	// Return a no-op command; tests never call Run.
	return exec.CommandContext(ctx, "true")
}

func (r *recordingExecer) PTYCommandContext(ctx context.Context, cmd string, args ...string) *pty.Cmd {
	r.commands = append(r.commands, append([]string{cmd}, args...))
	return &pty.Cmd{Context: ctx, Path: cmd, Args: append([]string{cmd}, args...)}
}

// TestCommandEnvExecer_NoShellInterpolation asserts that arguments forwarded
// through commandEnvExecer reach the wrapped execer as separate argv entries.
func TestCommandEnvExecer_NoShellInterpolation(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows PATH resolution uses exec.LookPath; not tested here.")
	}

	tempDir := t.TempDir()
	dockerPath := filepath.Join(tempDir, "docker")
	require.NoError(t, os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0o600))
	require.NoError(t, os.Chmod(dockerPath, 0o755))

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	commandEnv := func(usershell.EnvInfoer, []string) (string, string, []string, error) {
		return "/bin/bash", "/tmp", []string{"PATH=" + tempDir, "CUSTOM=value"}, nil
	}

	cases := []struct {
		name string
		args []string
	}{
		{name: "CommandSubstitution", args: []string{"exec", "--env", `FOO=$(touch /tmp/pwn)`, "ctr"}},
		{name: "Backticks", args: []string{"exec", "--env", "FOO=`id`", "ctr"}},
		{name: "VariableExpansion", args: []string{"exec", "--env", "FOO=$HOME", "ctr"}},
		{name: "Semicolon", args: []string{"inspect", "ctr; rm -rf /"}},
		{name: "Pipe", args: []string{"inspect", "ctr | curl evil"}},
		{name: "SingleQuotes", args: []string{"inspect", `it's`}},
		{name: "DoubleQuotes", args: []string{"inspect", `"quoted"`}},
		{name: "DockerTemplate", args: []string{"inspect", "--format", "{{.Config.Image}}", "ctr"}},
		{name: "EmptyArg", args: []string{"inspect", "", "ctr"}},
		{name: "Newline", args: []string{"inspect", "line1\nline2"}},
	}

	ctx := t.Context()
	for _, tc := range cases {
		t.Run("CommandContext/"+tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingExecer{}
			e := newCommandEnvExecer(logger, commandEnv, rec)
			_ = e.CommandContext(ctx, "docker", tc.args...)
			require.Len(t, rec.commands, 1)
			got := rec.commands[0]
			require.Equal(t, dockerPath, got[0],
				"first forwarded element must be the resolved docker path, never /bin/bash or a shell flag")
			require.Equal(t, tc.args, got[1:],
				"remaining argv must equal the original args verbatim")
		})
		t.Run("PTYCommandContext/"+tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &recordingExecer{}
			e := newCommandEnvExecer(logger, commandEnv, rec)
			_ = e.PTYCommandContext(ctx, "docker", tc.args...)
			require.Len(t, rec.commands, 1)
			got := rec.commands[0]
			require.Equal(t, dockerPath, got[0])
			require.Equal(t, tc.args, got[1:])
		})
	}
}

// TestCommandEnvExecer_SetsEnvAndDir asserts that env and dir from CommandEnv
// are applied to the returned command unchanged.
func TestCommandEnvExecer_SetsEnvAndDir(t *testing.T) {
	t.Parallel()

	wantEnv := []string{"PATH=/nonexistent", "FOO=bar"}
	wantDir := t.TempDir()
	commandEnv := func(usershell.EnvInfoer, []string) (string, string, []string, error) {
		return "/bin/bash", wantDir, wantEnv, nil
	}
	rec := &recordingExecer{}
	e := newCommandEnvExecer(slogtest.Make(t, nil), commandEnv, rec)

	c := e.CommandContext(t.Context(), "docker", "ps")
	require.Equal(t, wantEnv, c.Env)
	require.Equal(t, wantDir, c.Dir)
}

// TestCommandEnvExecer_CommandEnvError ensures the original cmd/args are
// forwarded with empty dir and nil env when CommandEnv fails.
func TestCommandEnvExecer_CommandEnvError(t *testing.T) {
	t.Parallel()

	commandEnv := func(usershell.EnvInfoer, []string) (string, string, []string, error) {
		return "", "", nil, errBoom
	}
	rec := &recordingExecer{}
	e := newCommandEnvExecer(slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}), commandEnv, rec)

	c := e.CommandContext(t.Context(), "docker", "ps", "--all")
	require.Len(t, rec.commands, 1)
	require.Equal(t, []string{"docker", "ps", "--all"}, rec.commands[0])
	require.Equal(t, "", c.Dir)
	require.Nil(t, c.Env)
}

var errBoom = &exec.Error{Name: "commandEnv", Err: exec.ErrNotFound}

func TestLookPathInEnv(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows PATH resolution uses exec.LookPath; not tested here.")
	}

	noExecDir := t.TempDir()
	execDir := t.TempDir()
	// noExecDir contains: a non-executable "tool", a directory named "alsotool".
	// execDir contains: an executable "tool" and an executable "alsotool".
	require.NoError(t, os.WriteFile(filepath.Join(noExecDir, "tool"), []byte("not exec"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(noExecDir, "alsotool"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "tool"), []byte("#!/bin/sh\nexit 0\n"), 0o600))
	require.NoError(t, os.Chmod(filepath.Join(execDir, "tool"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(execDir, "alsotool"), []byte("#!/bin/sh\nexit 0\n"), 0o600))
	require.NoError(t, os.Chmod(filepath.Join(execDir, "alsotool"), 0o755))

	tests := []struct {
		name    string
		nameArg string
		env     []string
		want    string
		wantErr bool
	}{
		{
			name:    "AbsolutePathPassThrough",
			nameArg: "/usr/bin/whatever",
			env:     []string{"PATH=/nonexistent"},
			want:    "/usr/bin/whatever",
		},
		{
			name:    "RelativePathPassThrough",
			nameArg: "./tool",
			env:     []string{"PATH=" + execDir},
			want:    "./tool",
		},
		{
			name:    "FoundInLaterPATHEntry",
			nameArg: "tool",
			env:     []string{"PATH=" + noExecDir + string(os.PathListSeparator) + execDir},
			want:    filepath.Join(execDir, "tool"),
		},
		{
			name:    "SkipsNonExecutable",
			nameArg: "tool",
			env:     []string{"PATH=" + noExecDir},
			wantErr: true,
		},
		{
			name:    "SkipsDirectory",
			nameArg: "alsotool",
			env:     []string{"PATH=" + noExecDir + string(os.PathListSeparator) + execDir},
			want:    filepath.Join(execDir, "alsotool"),
		},
		{
			name:    "MissingPATH",
			nameArg: "tool",
			env:     []string{"OTHER=value"},
			wantErr: true,
		},
		{
			name:    "EmptyPATH",
			nameArg: "tool",
			env:     []string{"PATH="},
			wantErr: true,
		},
		{
			name:    "EmptyPATHEntry",
			nameArg: "tool",
			env:     []string{"PATH=" + string(os.PathListSeparator) + execDir},
			want:    filepath.Join(execDir, "tool"),
		},
		{
			name:    "PATHLastDefinitionWins",
			nameArg: "tool",
			env:     []string{"PATH=/nonexistent", "PATH=" + execDir},
			want:    filepath.Join(execDir, "tool"),
		},
		{
			name:    "EmptyName",
			nameArg: "",
			env:     []string{"PATH=" + execDir},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := lookPathInEnv(tc.nameArg, tc.env)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

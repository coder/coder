//go:build linux

package agentnative

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"

	"github.com/coder/coder/v2/testutil"
)

func TestOpenCodeAttachAndOwnedShutdown(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	s := f.create(t, o)
	before, err := os.ReadFile(o.server.descriptorPath())
	require.NoError(t, err)
	command, err := o.Attach(t.Context(), s)
	require.NoError(t, err)
	require.Equal(t, "/test/opencode", command.Path)
	require.Equal(t, []string{"--server", f.descriptor.URL, "--session", s.NativeID, s.Cwd}, command.Args)
	require.Contains(t, command.Env, "OPENCODE_PASSWORD="+f.descriptor.Password)
	require.Contains(t, command.Env, "HOME="+filepath.Join(f.directory, "opencode-home"))
	for _, value := range command.Env {
		require.NotContains(t, value, "private-secret")
	}
	for _, value := range command.Args {
		require.NotContains(t, value, f.descriptor.Password)
	}
	require.NoError(t, o.Close())
	after, err := os.ReadFile(o.server.descriptorPath())
	require.NoError(t, err)
	require.Equal(t, before, after)
	f.mu.Lock()
	signals := f.signals
	f.mu.Unlock()
	require.Zero(t, signals)
	// Shutdown is independent of observer lifetime, but is never a broad stop.
	require.NoError(t, o.Shutdown(t.Context()))
	f.mu.Lock()
	signals = f.signals
	f.mu.Unlock()
	require.Equal(t, 1, signals)
	_, err = os.Stat(o.server.descriptorPath())
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, o.Shutdown(t.Context()))
}

func TestOpenCodeLifecycleRefusesAmbiguousOwnership(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"domain", "marker", "start", "listener", "unrecorded", "version", "symlink", "hardlink", "permissions"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newOpenCodeTestNative(t)
			o := f.backend(t)
			d := f.descriptor
			switch kind {
			case "domain":
				o.server.domain = func() (string, error) { return "other-domain", nil }
			case "marker", "start", "listener":
				original := o.server.process
				o.server.process = func(pid int) (*openCodeProcess, error) {
					p, err := original(pid)
					if kind == "marker" {
						p.Marker = "foreign"
					}
					if kind == "start" {
						p.StartTime = "reused"
					}
					if kind == "listener" {
						p.Ports = nil
					}
					return p, err
				}
			case "unrecorded":
				d.PID = 0
				d.StartTime = ""
				d.Status = "starting"
				require.NoError(t, o.server.writeDescriptor(d))
			case "version":
				d.Version = "2.0.29"
				require.NoError(t, o.server.writeDescriptor(d))
			case "symlink":
				require.NoError(t, os.Rename(o.server.descriptorPath(), filepath.Join(f.directory, "target")))
				require.NoError(t, os.Symlink(filepath.Join(f.directory, "target"), o.server.descriptorPath()))
			case "hardlink":
				require.NoError(t, os.Link(o.server.descriptorPath(), filepath.Join(f.directory, "target")))
			case "permissions":
				require.NoError(t, os.Chmod(o.server.descriptorPath(), 0o644))
			}
			_, err := o.server.connect(t.Context())
			require.Error(t, err)
			require.Error(t, o.server.shutdown(t.Context()))
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Zero(t, f.signals)
			require.Empty(t, f.calls, "ownership must be checked before sending credentials")
		})
	}
}

func TestOpenCodeAttachNeverBootsMissingServer(t *testing.T) {
	t.Parallel()
	f := newOpenCodeTestNative(t)
	o := f.backend(t)
	require.NoError(t, os.Remove(o.server.descriptorPath()))
	_, err := o.Attach(t.Context(), Session{ID: "opencode:" + f.id, NativeID: f.id, Cwd: "/workspace", Harness: "opencode"})
	require.ErrorContains(t, err, "not ready")
	require.Empty(t, f.mutations())
	_, err = os.Stat(o.server.descriptorPath())
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOpenCodeCLIProbeIsolationAndBound(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"valid", "version", "large"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			o, err := NewOpenCode(BackendOptions{StateDir: t.TempDir(), Host: &openCodeTestHost{}, Clock: quartz.NewMock(t), Environment: []string{"PATH=/bin", "HOME=/private-home", "OPENCODE_DB=private-secret", "OPENCODE_CONFIG_CONTENT=private-secret"}, Command: func(ctx context.Context, path string, args ...string) *exec.Cmd {
				require.Equal(t, "opencode", path)
				require.Equal(t, []string{"--version"}, args)
				script := `printf 'opencode v2.0.28\n'`
				if kind == "version" {
					script = `printf 'private-secret\n'`
				}
				if kind == "large" {
					script = `head -c 2048 /dev/zero`
				}
				cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
				// The adapter supplies Env and Dir after this factory returns.
				cmd.Cancel = func() error { return cmd.Process.Kill() }
				return cmd
			}})
			require.NoError(t, err)
			t.Cleanup(func() { _ = o.Close() })
			// Inspect the factory's assigned environment when the process starts.
			original := o.server.options.Command
			o.server.options.Command = func(ctx context.Context, path string, args ...string) *exec.Cmd {
				cmd := original(ctx, path, args...)
				cmd.Args = append(cmd.Args[:2], `test "$HOME" != /private-home && test -z "$OPENCODE_DB" && test "$PWD" = "$HOME" && `+cmd.Args[2])
				return cmd
			}
			_, err = o.server.checkVersion(t.Context())
			if kind == "valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-secret")
				if kind == "large" {
					require.ErrorContains(t, err, "version check failed")
				}
			}
		})
	}
}

// This helper is a real, detached HTTP child. Its environment and kernel socket
// ownership exercise production /proc and pidfd code without a native CLI.
func TestOpenCodeServerProcessHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv("CODER_OPENCODE_TEST_HELPER") != "1" {
		return
	}
	port := os.Getenv("CODER_OPENCODE_TEST_PORT")
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		_, password, _ := r.BasicAuth()
		if password != os.Getenv("OPENCODE_PASSWORD") {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": openCodeVersion, "pid": os.Getpid(), "urls": []string{"http://127.0.0.1:" + port}})
	})
	ready := os.NewFile(3, "ready")
	_, _ = ready.Write([]byte{1})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: testutil.WaitLong}
	_ = server.Serve(listener)
}

func TestOpenCodeRealProcessLifecycle(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"ready", "recover-starting", "shutdown-starting"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			checkOpenCodeProcessLifecycle(t, kind)
		})
	}
}

func checkOpenCodeProcessLifecycle(t *testing.T, kind string) {
	t.Helper()
	clock := quartz.NewMock(t)
	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	defer cancel()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	defer writer.Close()
	ready, exited := make(chan struct{}), make(chan struct{})
	go func() {
		var marker [1]byte
		_, _ = io.ReadFull(reader, marker[:])
		close(ready)
		_, _ = io.Copy(io.Discard, reader)
		close(exited)
	}()
	options := BackendOptions{StateDir: t.TempDir(), Host: &openCodeTestHost{}, Clock: clock, Environment: []string{"HOME=/not-used", "PATH=/bin"}}
	require.NoError(t, os.Chmod(options.StateDir, 0o700))
	var child *exec.Cmd
	t.Cleanup(func() {
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
		}
	})
	options.Command = func(_ context.Context, path string, args ...string) *exec.Cmd {
		require.Equal(t, "/test/opencode", path)
		require.Equal(t, []string{"serve", "--hostname", "127.0.0.1", "--port"}, args[:4])
		port := args[len(args)-1]
		child = exec.Command("/bin/sh", "-c", `export CODER_OPENCODE_TEST_HELPER=1 CODER_OPENCODE_TEST_PORT="$1"; exec "$2" -test.run=^TestOpenCodeServerProcessHelper$`, "helper", port, os.Args[0])
		child.ExtraFiles = []*os.File{writer}
		return child
	}
	s := newOpenCodeServer(options, clock)
	s.version = func(context.Context) (string, error) { return "/test/opencode", nil }
	startupCtx, startupCancel := context.WithCancel(ctx)
	defer startupCancel()
	var closeParent sync.Once
	processCalls := 0
	s.process = func(pid int) (*openCodeProcess, error) {
		processCalls++
		if kind != "ready" && processCalls == 2 {
			// The first process read saves PID/start time. Cancel during the
			// first readiness check, leaving the healthy child in starting.
			startupCancel()
		}
		closeParent.Do(func() { _ = writer.Close() })
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return openCodeProcessIdentity(pid)
	}
	s.signal = func(d openCodeDescriptor) error {
		fd, err := unix.PidfdOpen(d.PID, 0)
		if err != nil {
			return err
		}
		defer unix.Close(fd)
		if fd < 0 || fd > math.MaxInt32 {
			return xerrors.New("helper pidfd is outside the poll descriptor range")
		}
		pollFD := int32(fd)
		if err := openCodeSignal(d); err != nil {
			return err
		}
		// Pipe EOF can precede the kernel's process-exit transition. Wait on
		// the exact pidfd rather than racing /proc or sleeping in this test.
		n, err := unix.Poll([]unix.PollFd{{Fd: pollFD, Events: unix.POLLIN}}, int(testutil.WaitLong/time.Millisecond))
		if err != nil {
			return err
		}
		if n != 1 {
			return xerrors.New("helper did not exit")
		}
		select {
		case <-exited:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	d, err := s.ensure(startupCtx)
	if kind == "ready" {
		require.NoError(t, err)
	} else {
		require.Error(t, err)
		stored, err := s.readDescriptor()
		require.NoError(t, err)
		require.NotNil(t, stored)
		d = *stored
		require.Equal(t, "starting", d.Status)
	}
	t.Cleanup(func() { _ = openCodeSignal(d) })
	require.Equal(t, child.Process.Pid, d.PID)
	require.Contains(t, child.Env, "HOME="+filepath.Join(options.StateDir, "opencode-home"))
	for _, arg := range child.Args {
		require.NotContains(t, arg, d.Password)
	}
	// A new observer reuses the same real process and never launches another.
	restarted := newOpenCodeServer(options, clock)
	restarted.version = func(context.Context) (string, error) {
		t.Fatal("healthy server reuse must not depend on a CLI probe")
		return "", nil
	}
	restarted.options.Command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("healthy server must be reused")
		return nil
	}
	connected, err := restarted.connect(ctx)
	require.NoError(t, err)
	require.Equal(t, d, connected, "read-only attachment can verify a starting server")
	if kind != "shutdown-starting" {
		reused, err := restarted.ensure(ctx)
		require.NoError(t, err)
		expected := d
		expected.Status = "ready"
		require.Equal(t, expected, reused)
		stored, err := restarted.readDescriptor()
		require.NoError(t, err)
		require.Equal(t, expected, *stored)
	}
	restarted.signal = s.signal
	require.NoError(t, restarted.shutdown(ctx))
	_, err = os.Stat(s.descriptorPath())
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOpenCodeSocketIdentity(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	process, err := openCodeProcessIdentity(os.Getpid())
	require.NoError(t, err)
	require.NotNil(t, process)
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	require.Contains(t, process.Ports, port)
	domain, err := openCodeProcessDomain()
	require.NoError(t, err)
	require.Contains(t, domain, "pid:[")
}

func TestOpenCodeNativeEnvironmentIsolation(t *testing.T) {
	t.Parallel()
	original := []string{"HOME=/user", "PATH=/bin", "OPENCODE_DB=private", "OPENCODE_PASSWORD=private", "OPENCODE_CONFIG_CONTENT=private", "OPENCODE_TEST_HOME=private", "OPENCODE_DISABLE_MODELS_FETCH=true", "ANTHROPIC_API_KEY=user-key"}
	environment := openCodeEnvironment("/native", original)
	for _, item := range environment {
		require.False(t, strings.Contains(item, "private"), fmt.Sprint(environment))
	}
	require.Contains(t, environment, "OPENCODE_DISABLE_MODELS_FETCH=true")
	require.Contains(t, environment, "ANTHROPIC_API_KEY=user-key")
	require.Contains(t, environment, `OPENCODE_CONFIG_CONTENT={"update":"disable"}`)
	require.Equal(t, "HOME=/user", original[0])
}
